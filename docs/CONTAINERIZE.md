# Containerize OTUI

![Containerize](containerize.png)                                                                                

Users may want to sandbox OTUI or run multiple OTUI instances simultaneously on the same host.

This is a short guide to run OTUI in OCI containers to achieve the above two use cases.

### How?
---

Docker:

```
docker run -ti --rm \
                -v </path/to/config/dir/on/host>:/home/otui/.config/otui \ 
                -v </path/to/profile/dir/on/host>:/home/otui/.local/share/otui \ 
                -v </path/to/encryption/dir/on/host>:/home/otui/.ssh \
                ghcr.io/hkdb/otui:v0.03.01
```
You will notice that there are 3 volumes mounted so that your data persists. Config, Data (Profile), and SSH (to encrypt your API keys at rest). For more information, see the configuration section of the `README.md` at the [Github Repo](https://github.com/hkdb/otui).

This command launches OTUI in a container. Once you quit OTUI, it will stop and remove the container from your system. The only thing left on the host are the volume directories on the host which you can delete if you don't want it on your system anymore or keep it there for the next time the container launch so you can start right where you left off.

Of course, the command is super long so to make it easier, create an alias:

```bash
alias otui='docker run -ti --rm -v </path/to/config/dir/on/host>:/home/otui/.config/otui -v </path/to/profile/dir/on/host>:/home/otui/.local/share/otui -v </path/to/encryption/dir/on/host>:/home/otui/.ssh ghcr.io/hkdb/otui:v0.03.01'
```
That way, you can just type `otui` in the terminal instead.

Incus, LXD, Podman:

If you are using these alternatives, chances are, you already know what you are doing and can adjust your solution based on the above.

### Why?
---

OTUI is designed to be a single instance application on any given host to ensure that there are no race conditions when more than one instance is trying to write to the same session/profile.

Furthermore, MCP plugins are launched per instance of OTUI so if you have 2 instances of OTUI running, there might be MCP servers that are trying to use the same port on localhost.

Until more sophisticated multi-instance handling is implemented, the most effective way to run multiple instances of OTUI on a single host is to run each instance with a container.

Furthermore, because MCPs make LLM models extremely powerful, it can cause a lot of trouble if the model makes a mistake doing the wrong thing to your system so having some sort of sandboxing can be beneficial.

Last but not least, You may just want to try OTUI without installing it on your system which running it in a container is the perfect way to go about it.


[<< Return to OTUI site 🌐](https://hkdb.github.io/otui)

[<< Return to OTUI repo 🕸️](https://github.com/hkdb/otui)
