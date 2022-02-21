output "nic1_ip_address" {
 value = aws_instance.ec2_instance.private_ip
}

output "instance_id" {
 value = aws_instance.ec2_instance.id
}

output "pub_ip_address" {
 value = aws_instance.ec2_instance.public_ip
}

output "pub_dns_name" {
 value = aws_instance.ec2_instance.public_dns
}