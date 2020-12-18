# am2gotify

A 200 LoC [socket-activated](https://www.freedesktop.org/software/systemd/man/systemd.socket.html)
bridge listening for
[Prometheus Alertmanager](https://prometheus.io/docs/alerting/latest/alertmanager/)
webhooks and forwarding them as [Gotify](https://gotify.net/) notifications.

### Usage

1. Register an *Application* in your Gotify server and note its token.
1. Optionally, if you want to auto-delete resolved alerts (`--resolved=delete`), also register a *Client* in Gotify and note its token

```
$ am2gotify   --url       https://url/to/gotify/server \
              --token     <application token> \
              --resolved  {notify,ignore,delete} \
            [ --ctoken    <client token, required for --resolved=delete> ]
            [ --exitafter <seconds> ]
```

For integration, see the example systemd [`socket`](./am2gotify.socket) and [`service`](./am2gotify.service) files.

### Prior art

* [Refused feature request for adding Gotify to upstream Alertmanager](https://github.com/prometheus/alertmanager/issues/2120)
* [alertify](https://github.com/scottwallacesh/alertify), Python, over-engineered
* [alertmanager-gotify](https://github.com/scottwallacesh/alertmanager-gotify), Go, unusable by lack of configuration
* [alertmanager-notifier](https://github.com/ix-ai/alertmanager-notifier), Python, too Docker-centric

### License

MIT.
