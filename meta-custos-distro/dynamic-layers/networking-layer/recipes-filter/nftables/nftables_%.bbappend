# Custos policy: no Python on the device. The python PACKAGECONFIG only
# builds the nftables-python bindings; nft and libnftables do not need it.
PACKAGECONFIG:remove = "python"
