output "bdds_nic1_ip_address" { value = module.BDDS.nic1_ip_address }
output "bdds_instance_id"    { value = module.BDDS.instance_id }
output "bdds_pub_ip_address"  { value = module.BDDS.pub_ip_address }
output "bdds_pub_dns_name"    { value = module.BDDS.pub_dns_name }

output "bam_nic1_ip_address" { value = module.BAM.nic1_ip_address }
output "bam_instance_id"    { value = module.BAM.instance_id }
output "bam_pub_ip_address"  { value = module.BAM.pub_ip_address }
output "bam_pub_dns_name"    { value = module.BAM.pub_dns_name }

output "gw_nic1_ip_address" { value = module.Gateway.nic1_ip_address }
output "gw_instance_id"    { value = module.Gateway.instance_id }
output "gw_pub_ip_address"  { value = module.Gateway.pub_ip_address }
output "gw_pub_dns_name"    { value = module.Gateway.pub_dns_name }

output "BAM_IP_Public"    { value = module.BAM.pub_ip_address }
output "BAM_IP_Private"   { value = module.BAM.nic1_ip_address }
output "BDDS_IP_Public"   { value = module.BDDS.pub_ip_address }
output "BDDS_IP_Private"  { value = module.BDDS.nic1_ip_address }
output "Gateway_IP"       { value = module.Gateway.pub_ip_address }
output "BAM_Password"     { value = module.BAM.instance_id }
