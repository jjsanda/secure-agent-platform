import { Home } from 'lucide-react'
import { Link } from 'react-router-dom'

import { Button } from '@/components/ui/button'

export function NotFound() {
  return (
    <div className="flex flex-col items-center justify-center py-24 text-center">
      <p className="text-5xl font-semibold text-primary">404</p>
      <h1 className="mt-3 text-lg font-semibold">Page not found</h1>
      <p className="mt-1 text-sm text-muted-foreground">
        The page you are looking for does not exist.
      </p>
      <Button asChild className="mt-6">
        <Link to="/">
          <Home className="size-4" />
          Back to overview
        </Link>
      </Button>
    </div>
  )
}
