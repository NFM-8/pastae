# Pastae

Encrypting in-memory paste service.

# Features

* Fully in-memory for ephemerality and performance

* AEAD-encryption with per-paste random key and nonce using 128 bit AES-GCM

* Burn after reading, no explicit overwriting is neede because of the encryption

* Additional scrambling of encryption keys to defend against current and future SPECTRE and MELTDOWN type attacks in hosted deployments