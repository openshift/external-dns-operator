variable "ami_id" {}
variable "private_ip" {}
variable "subnet_id" {}
variable "vpc_security_group_id" {}
variable "key_pair_name" {}
variable "vm_name" {}
variable "instance_type" {}
variable "public" { default = false }

variable "cloud_config" {}
