resource "aws_instance" "ec2_instance" {
  ami = var.ami_id
  instance_type = var.inst_type
  key_name = var.keypair_name
  subnet_id = var.subnet_id
  private_ip = var.private_ip
  vpc_security_group_ids = [var.vpc_security_group_id]
  associate_public_ip_address = var.public
  tags = {
    Name = var.vm_name
  }

  provisioner "file" {
    source = "${path.module}/scripts/install_gw.sh"
    destination = "/tmp/install_gw.sh"

    connection {
      type     = "ssh"
      user     = "ubuntu"
      private_key = file("${path.root}/files/${var.pem_file}")
      host     = aws_instance.ec2_instance.public_ip
    }
  }

  provisioner "file" {
    source = "${path.module}/scripts/start_gw.sh"
    destination = "/tmp/start_gw.sh"

    connection {
      type     = "ssh"
      user     = "ubuntu"
      private_key = file("${path.root}/files/${var.pem_file}")
      host     = aws_instance.ec2_instance.public_ip
    }
  }

  provisioner "remote-exec" {
    inline = [
      "sudo /bin/sh /tmp/install_gw.sh -b ${var.bam_ip} -u ${var.username}"
    ]

    connection {
      type     = "ssh"
      user     = "ubuntu"
      private_key = file("${path.root}/files/${var.pem_file}")
      host     = aws_instance.ec2_instance.public_ip
    }
  }

  provisioner "remote-exec" {
    inline = [
      "/bin/bash /tmp/start_gw.sh -b ${var.bam_ip} -u ${var.username}"
    ]

    connection {
      type     = "ssh"
      user     = "ubuntu"
      private_key = file("${path.root}/files/${var.pem_file}")
      host     = aws_instance.ec2_instance.public_ip
    }
  }
}
