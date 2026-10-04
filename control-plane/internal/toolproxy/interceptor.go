// Package toolproxy implements the ToolProxyService: the single, audited choke
// point through which the agent worker runs tools. The worker presents its
// scoped credential as gRPC metadata; a server interceptor validates it (and
// checks the tenant's revocation store) BEFORE the handler runs, so a request
// with a bad, expired, or revoked credential never reaches a tool.
package toolproxy

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/jjsanda/secure-agent-platform/control-plane/internal/credential"
	dbgen "github.com/jjsanda/secure-agent-platform/control-plane/internal/db/gen"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/telemetry"
	"github.com/jjsanda/secure-agent-platform/control-plane/internal/tenancy"
)

// MetadataKey is the gRPC metadata key carrying the PASETO scoped credential.
const MetadataKey = "x-scoped-credential"

type scopedCtxKey struct{}

func scopedFrom(ctx context.Context) (credential.Scoped, bool) {
	s, ok := ctx.Value(scopedCtxKey{}).(credential.Scoped)
	return s, ok
}

var errRevoked = errors.New("credential revoked")

// CredentialInterceptor authorizes every unary ToolProxy call before its handler:
//  1. extract the credential from gRPC metadata (missing -> Unauthenticated)
//  2. verify signature / audience / issuer / time bounds (invalid -> Unauthenticated)
//  3. confirm the jti is present and unrevoked in the credential's tenant store,
//     and stamp last_used_at (absent/revoked -> PermissionDenied)
//
// The decoded capability is injected into the context for the handler. Note that
// step 3 runs inside the token's OWN tenant scope, so the revocation lookup is
// itself protected by row-level security.
func CredentialInterceptor(verifier *credential.Verifier, store *tenancy.Store, metrics *telemetry.Metrics) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			metrics.CredentialDenied(ctx, "no_metadata")
			return nil, status.Error(codes.Unauthenticated, "missing request metadata")
		}
		vals := md.Get(MetadataKey)
		if len(vals) == 0 || vals[0] == "" {
			metrics.CredentialDenied(ctx, "missing")
			return nil, status.Error(codes.Unauthenticated, "missing scoped credential")
		}

		scoped, err := verifier.Verify(vals[0], time.Now())
		if err != nil {
			metrics.CredentialDenied(ctx, "invalid")
			return nil, status.Error(codes.Unauthenticated, "invalid scoped credential")
		}

		if err := store.WithTenant(ctx, scoped.TenantID, func(q *dbgen.Queries) error {
			rec, err := q.GetScopedCredential(ctx, scoped.JTI)
			if err != nil {
				return err // not found: unknown jti, or filtered out by RLS
			}
			if rec.RevokedAt.Valid {
				return errRevoked
			}
			return q.TouchScopedCredential(ctx, scoped.JTI)
		}); err != nil {
			metrics.CredentialDenied(ctx, "revoked_or_unknown")
			return nil, status.Error(codes.PermissionDenied, "credential is unknown or revoked")
		}

		return handler(context.WithValue(ctx, scopedCtxKey{}, scoped), req)
	}
}
