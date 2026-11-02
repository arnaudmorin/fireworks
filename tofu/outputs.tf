output "ssh_command" {
  description = "Run this to watch the show"
  value       = "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR fireworks@${openstack_compute_instance_v2.fireworks.access_ip_v4}"
}
