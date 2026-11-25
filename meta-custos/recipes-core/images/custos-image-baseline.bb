SUMMARY = "Custos baseline image (demo comparison, NOT for use)"
DESCRIPTION = "The same recipes as custos-image without the run-time \
hardening and without custos-watch, so the demo scripts (tests/demo) can \
show baseline vs hardened. A10 will add more differences (verity, signed \
FIT) once those exist."

require custos-image.bb

CUSTOS_HARDENING = ""
IMAGE_INSTALL:remove = "custos-watch"
