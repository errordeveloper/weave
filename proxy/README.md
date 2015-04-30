# Weave Docker proxy

The Weave Docker proxy provides a convenient way to integrate weave
with Docker; in particular, it means you can start containers using
the docker client (or other docker API-using tool) and have them
attach to a weave network.

## Example

To start things off, it's necessary to launch weave and the proxy:

```bash
$ weave launch
$ weave launch-proxy
```

> This starts the proxy container, which will listen on the host's IP
> for docker commands. The proxy container has everything it needs for
> executing weave operations.

This makes the proxy available on `<host>:12375`.

You can now talk to the proxy as though it were the Docker daemon
itself. Containers with an environment entry of `WEAVE_CIDR` will be
given an IP address on the weave network.

```bash
$ export DOCKER_HOST=tcp://localhost:12375
$ docker run -e WEAVE_CIDR=10.2.1.5/24 -ti ubuntu
```

The weave script provides a convenience in the form of `weave run`,
which will make sure the proxy is running, then use it to start the
given container on the weave network.

```bash
$ weave run 10.3.5.6/24 -ti --name=foo ubuntu
```
