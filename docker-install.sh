#!/bin/bash

set -e

echo "======================================="
echo " Docker Installation for Ubuntu 24.04"
echo "======================================="

# Ensure script is run as root
if [ "$EUID" -ne 0 ]; then
    echo "Please run as root:"
    echo "sudo ./install-docker.sh"
    exit 1
fi

echo
echo "==> Removing old Docker packages..."

for pkg in docker.io docker-doc docker-compose docker-compose-v2 podman-docker containerd runc; do
    apt remove -y "$pkg" >/dev/null 2>&1 || true
done

echo
echo "==> Updating package lists..."

apt update

echo
echo "==> Installing prerequisites..."

apt install -y \
    ca-certificates \
    curl \
    gnupg

echo
echo "==> Adding Docker GPG key..."

install -m 0755 -d /etc/apt/keyrings

curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
    | gpg --dearmor -o /etc/apt/keyrings/docker.gpg

chmod a+r /etc/apt/keyrings/docker.gpg

echo
echo "==> Adding Docker repository..."

echo \
"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
https://download.docker.com/linux/ubuntu \
$(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
| tee /etc/apt/sources.list.d/docker.list >/dev/null

echo
echo "==> Updating package lists..."

apt update

echo
echo "==> Installing Docker..."

apt install -y \
    docker-ce \
    docker-ce-cli \
    containerd.io \
    docker-buildx-plugin \
    docker-compose-plugin

echo
echo "==> Enabling Docker..."

systemctl enable docker
systemctl start docker

echo
echo "==> Docker Version"
docker --version

echo
echo "==> Docker Compose Version"
docker compose version

echo
echo "==> Docker Status"
systemctl --no-pager --full status docker

echo
echo "======================================="
echo "Docker installation completed."
echo "======================================="

echo
echo "To allow your normal user to run Docker without sudo:"
echo
echo "  sudo usermod -aG docker \$USER"
echo
echo "Then log out and back in (or run 'newgrp docker')."