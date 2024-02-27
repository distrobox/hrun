# hrun

Run commands on your host machine from inside your [distrobox](https://github.com/89luca89/distrobox)
or [toolbx](https://github.com/containers/toolbox) container.

Highly inspired by [host-spawn](https://github.com/1player/host-spawn).

> [!WARNING]  
> This is a work in progress and not yet ready for production use. Expect
> breaking changes and the worst possible bugs. A LOT of features are missing,
> the code is not optimized, security is not a concern and the documentation is
> incomplete. Good luck!

## Usage

First you have to start the socket server on your host machine:

```bash
hrun start
```

Then you can run commands from inside your container:

```bash
hrun echo "Hello from your host machine"
```

## What's the point?

The main difference between `hrun` and `host-spawn` is that `hrun` relies on a
socket server running on the host machine, while `host-spawn` uses the Flatpak
DBus to communicate with the host, introducing 2 big dependencies.

This is not a replacement for `host-spawn`, but a different approach to the same
problem on those systems where Flatpak or DBus are not available.
