# eth-diff-fuzz

A fast & easy to maintain differential fuzzer for Ethereum clients.

## System configuration

This fuzzer works with Linux and macOS, not Windows. This is because it uses Unix Domain Sockets and
Shared Memory segments for interprocess communication.

First, two limits must be raised to support 100 MiB segements.

* `shmmax` -- the max shared memory segment size.
* `shmall` -- total shared memory size in pages.

### Linux

```bash
sudo sysctl -w kernel.shmmax=104857600
sudo sysctl -w kernel.shmall=256000
```

### macOS

```bash
sudo sysctl -w kern.sysv.shmmax=104857600
sudo sysctl -w kern.sysv.shmall=256000
```

## Drivers

The *driver* is the central component that communicates with *processors* (clients).

Some of the driver's responsibilities:

* Create unix socket & shared memory segments.
* Handle processor registrations & disconnections.
* Generate random inputs to share with processors.
* Check that processor responses are the same.

To run a driver:

```bash
cd drivers/consensus
go run .
```

## Processors

A *processor* is the component which takes some input, processes it, and returns it to the driver.
These must be run on the same system as the driver, but can be written in any programming language.

### Golang

```bash
cd processors/golang
go run .
```

### Java

```bash
cd processors/java
make
```

### Rust

```bash
cd processors/rust
cargo run
```
