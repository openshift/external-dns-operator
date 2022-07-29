terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 3.0"
    }
  }
}

provider "aws" {
  region = var.region
  access_key = var.access_key
  secret_key = var.access_secret
}

module "BDDS" {
  source 		            = "./modules/bluecat_bdds"
  vm_name               = "BDDS"
  ami_id                = var.ami_bdds
  instance_type         = "t2.large"
  subnet_id             = aws_subnet.main.id
  private_ip            = "10.10.1.30"
  vpc_security_group_id = aws_security_group.bdds.id
  key_pair_name         = var.keypair
  public                = true
  cloud_config          = file("./files/bdds_license.txt")
}

module "BAM" {
  source 		            = "./modules/bluecat_bam"
  vm_name               = "BAM"
  ami_id                = var.ami_bam
  instance_type         = "c4.xlarge"
  subnet_id             = aws_subnet.main.id
  private_ip            = "10.10.1.20"
  vpc_security_group_id = aws_security_group.bam.id
  key_pair_name         = var.keypair
  public                = true
  cloud_config          = file("./files/bam_license.txt")
}

module "Gateway" {
  source 		            = "./modules/bluecat_gateway"
  vm_name               = "Gateway_Server"
  ami_id                = var.ami_gw
  inst_type             = "t2.micro"
  keypair_name          = var.keypair
  pem_file              = var.keypair_file
  subnet_id             = aws_subnet.main.id
  private_ip            = "10.10.1.50"
  vpc_security_group_id = aws_security_group.gw.id
  public                = true
  bam_ip                = module.BAM.nic1_ip_address
  username              = "ubuntu"
}
