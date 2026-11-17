# EC2 instance for FlashLive, following EC2-DEPLOY.md.
#
# Creates:
#   - a security group with the inbound ports from section 2 of the guide
#   - one c7i.flex.large instance (x86) with a 30 GB gp3 root volume
#   - an Elastic IP attached to the instance (point your DuckDNS name at it)
#
# Usage:
#   terraform init
#   terraform apply -var 'ssh_cidr=<your public IP>/32'
#
# The key pair "LiveShare" must already exist in the chosen region (its private
# key is LiveShare.pem). Key pairs are per region.
#
# Changing region: destroy the old deployment first, passing the OLD region
# explicitly, e.g.
#   terraform destroy -var 'region=ap-southeast-2' -var 'ssh_cidr=0.0.0.0/32'
# Running destroy or apply against a different region than the one the state
# was created in makes Terraform forget the old resources without deleting them.

terraform {
  required_version = ">= 1.5.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
}

provider "aws" {
  region = var.region
}

variable "region" {
  description = "AWS region to deploy into."
  type        = string
  # The account's organization policy denies EC2 in other regions (checked
  # for ap-southeast-1), so this stays on Sydney.
  default = "ap-southeast-2" # Sydney
}

variable "instance_type" {
  description = "EC2 instance type (x86, 2 vCPU). Use t3.medium if this type is not offered in the region."
  type        = string
  default     = "c7i-flex.large"
}

variable "ami_id" {
  description = "AMI to use. Leave null to look up the latest Ubuntu Server 24.04 LTS (x86_64) AMI for the region."
  type        = string
  default     = null
}

# Latest Ubuntu Server 24.04 LTS for x86_64, published by Canonical. AMI ids
# differ per region, so this is looked up instead of hard-coded.
data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"] # Canonical

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

variable "key_name" {
  description = "Name of an existing EC2 key pair in this region. Its private key file is assumed to be <key_name>.pem."
  type        = string
  default     = "LiveShare"
}

variable "ssh_cidr" {
  description = "CIDR allowed to SSH (port 22), e.g. 203.0.113.10/32. Keep this to your own IP."
  type        = string
}

variable "root_volume_size" {
  description = "Root volume size in GB (gp3)."
  type        = number
  default     = 30
}

variable "name" {
  description = "Name prefix for the resources."
  type        = string
  default     = "flashlive"
}

data "aws_vpc" "default" {
  default = true
}

resource "aws_security_group" "flashlive" {
  name        = "${var.name}-sg"
  description = "FlashLive EC2 inbound rules (EC2-DEPLOY.md section 2)"
  vpc_id      = data.aws_vpc.default.id

  # SSH, limited to your own IP
  ingress {
    description = "SSH"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [var.ssh_cidr]
  }

  # Let's Encrypt validation and HTTP to HTTPS redirect
  ingress {
    description = "HTTP"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # Web app, API and signaling (wss)
  ingress {
    description = "HTTPS"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # TURN
  ingress {
    description = "TURN TCP"
    from_port   = 3478
    to_port     = 3478
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    description = "TURN UDP"
    from_port   = 3478
    to_port     = 3478
    protocol    = "udp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # TURN over TLS
  ingress {
    description = "TURN over TLS"
    from_port   = 5349
    to_port     = 5349
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # TURN relay media
  ingress {
    description = "TURN relay media"
    from_port   = 49152
    to_port     = 49200
    protocol    = "udp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # SFU media (WebRTC)
  ingress {
    description = "SFU media"
    from_port   = 50000
    to_port     = 50199
    protocol    = "udp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # Outbound: unrestricted (package installs, image pulls, Let's Encrypt)
  egress {
    description = "All outbound"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "${var.name}-sg"
  }
}

resource "aws_instance" "flashlive" {
  ami                    = coalesce(var.ami_id, data.aws_ami.ubuntu.id)
  instance_type          = var.instance_type
  key_name               = var.key_name
  vpc_security_group_ids = [aws_security_group.flashlive.id]

  # A newer Ubuntu image must not replace (and wipe) a running instance.
  lifecycle {
    ignore_changes = [ami]
  }

  root_block_device {
    volume_size = var.root_volume_size
    volume_type = "gp3"
  }

  tags = {
    Name = var.name
  }
}

resource "aws_eip" "flashlive" {
  domain = "vpc"

  tags = {
    Name = "${var.name}-eip"
  }
}

resource "aws_eip_association" "flashlive" {
  instance_id   = aws_instance.flashlive.id
  allocation_id = aws_eip.flashlive.id
}

output "elastic_ip" {
  description = "Public address. Set this as the IP of your DuckDNS subdomain."
  value       = aws_eip.flashlive.public_ip
}

output "ssh_command" {
  description = "SSH command from the guide (replace the key path)."
  value       = "ssh -i ~/keys/${var.key_name}.pem ubuntu@${aws_eip.flashlive.public_ip}"
}
