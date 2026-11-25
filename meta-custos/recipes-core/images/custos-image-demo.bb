SUMMARY = "Custos demo image for scenario S3 (DELIBERATELY VULNERABLE, lab only)"
DESCRIPTION = "custos-image with the same hardening, but custosd has one \
method without input validation (build tag vulnerable_demo). It shows that \
SELinux keeps a compromised custosd away from the WireGuard keys. Build: \
bitbake -R conf/custos-demo.conf custos-image-demo"

require custos-image.bb
