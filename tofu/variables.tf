variable "region" {
  type        = string
  default     = null
  description = "OpenStack region; defaults to OS_REGION_NAME from the environment."
}

variable "name" {
  type    = string
  default = "fireworks"
}

variable "image_name" {
  type    = string
  default = "Ubuntu 24.04"
}

variable "flavor_name" {
  type    = string
  default = "d2-2"
}

variable "network_name" {
  type    = string
  default = "Ext-Net"
}

variable "ssh_public_key_file" {
  type        = string
  default     = "~/.ssh/id_ed25519.pub"
  description = "Public key for the admin (ubuntu) account."
}

variable "message" {
  type        = string
  default     = null
  description = "Text written in sparks at the end of the show; null keeps the binary's default."

  validation {
    condition     = try(length(var.message), 0) <= 255
    error_message = "OpenStack metadata values are limited to 255 characters."
  }
}
