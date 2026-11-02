output "ssh_command" {
  description = "Run this to watch the show"
  value       = "ssh fireworks@${openstack_compute_instance_v2.fireworks.access_ip_v4}"
}
