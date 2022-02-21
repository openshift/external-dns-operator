variable "keypair" {}
variable "keypair_file" {}
variable "access_key" {}
variable "access_secret" {}
variable "vpc_name" { default = "bluecat" }
variable "subnet_name" { default = "bluecat" }
variable "ig_name" { default = "bluecat" }
variable "routes_name" { default = "bluecat" }
variable "vpc_cidr" { default = "10.10.1.0/24" }
variable "subnet_cidr" { default = "10.10.1.0/24" }
variable "region" { default = "us-east-1" }
variable "ami_bdds" {
    # 9.3.1, found in the subscription
    default = "ami-0f22ee2a815738fbf"
}
variable "ami_bam" {
    # 9.3.1, found in the subscription
    default = "ami-0bb9a9d9c44a18a9e"
}
variable "ami_gw" {
    default = "ami-01237fce26136c8cc"
}
