variable "vm_name" {}
variable "ami_id" {}
variable "inst_type" {}
variable "keypair_name" {}
variable "subnet_id" {}
variable "private_ip" {}
variable "vpc_security_group_id" {}
variable "pem_file" {}
variable "public" { default = false }

variable "bam_ip" {}
variable "username" { default = "ubuntu" }
