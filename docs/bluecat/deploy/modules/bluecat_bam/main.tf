resource "aws_instance" "ec2_instance" {
  ami = var.ami_id
  instance_type = var.instance_type
  key_name = var.key_pair_name
  private_ip = var.private_ip
  subnet_id = var.subnet_id
  vpc_security_group_ids = [var.vpc_security_group_id]
  associate_public_ip_address = var.public
  tags = {
    Name = var.vm_name
  }

  // Specific BDDS & BAM configuration information
  user_data_base64 = base64gzip(var.cloud_config)
}
