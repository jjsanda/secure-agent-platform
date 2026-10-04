# --- RDS Postgres in the private subnets, reachable only from the VPC -----------

resource "aws_db_subnet_group" "this" {
  name       = "${var.name}-db"
  subnet_ids = aws_subnet.private[*].id
  tags       = var.tags
}

resource "aws_security_group" "rds" {
  name        = "${var.name}-rds"
  description = "Postgres access from within the VPC only."
  vpc_id      = aws_vpc.this.id
  tags        = merge(var.tags, { Name = "${var.name}-rds" })

  ingress {
    description = "Postgres from the VPC (EKS nodes)."
    from_port   = 5432
    to_port     = 5432
    protocol    = "tcp"
    cidr_blocks = [var.vpc_cidr]
  }

  egress {
    description = "Allow all egress."
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_db_instance" "postgres" {
  identifier     = "${var.name}-pg"
  engine         = "postgres"
  engine_version = var.db_engine_version
  instance_class = var.db_instance_class

  allocated_storage = var.db_allocated_storage
  storage_encrypted = true

  db_name  = var.db_name
  username = var.db_username
  password = var.db_password

  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [aws_security_group.rds.id]

  multi_az                  = true
  backup_retention_period   = 7
  deletion_protection       = true
  skip_final_snapshot       = false
  final_snapshot_identifier = "${var.name}-pg-final"

  tags = var.tags
}
