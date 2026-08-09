# stammtisch

A self-hosted service for community voice and video. It aims at the
experience people expect from the commercial incumbent: switching between
voice channels without friction, rooms that stay open, a participant list
you can see before you join, per-person volume, and latency low enough that
conversation feels natural. A bot API is a first-class part of it, not an
afterthought.

The transport building blocks - WebRTC and selective forwarding - are open
source. What this project builds is the orchestration, the client, and the
relentless finish that turns those blocks into something a community would
actually leave a polished commercial product for.

Planning happens on the issue tracker first. Every decision that shapes the
architecture is written down there with its reasons before the code that
depends on it exists.

See [NOTICE.md](NOTICE.md) for the intended-use notice.

See [LICENSE](LICENSE) for the terms of the server, which is this repository
apart from the paths named below: the GNU Affero General Public License version
3 or later.

The surfaces a third party writes against are under the Apache License 2.0
instead, so that a bot or a client is not put under the server's terms by the
act of talking to it. That is [botapi/](botapi/) today, with its terms in
[botapi/LICENSE](botapi/LICENSE). Which paths those are is a list a check reads,
in [.github/check-licence-headers.sh](.github/check-licence-headers.sh), and the
check refuses a file carrying the identifier of the arm it is not under.

See [CONTRIBUTING.md](CONTRIBUTING.md) for how a change gets in, and which of
the rules there anything actually refuses.

See [GOVERNANCE.md](GOVERNANCE.md) for who decides and how a disagreement is
resolved, and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for what is expected of
people here and where to report behaviour that is not.

See [SECURITY.md](SECURITY.md) before reporting a vulnerability. It goes
through a private route rather than a public issue.
