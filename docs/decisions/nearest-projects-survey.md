# What the nearest open projects actually do today

Status: surveyed, 2026-08-06. Raised by issue #12.

## What was examined, and how

Three projects, each in a checkout at a named version. No running instance was
used. That bound applies to every cell below and is the reason so many of them
say the check could not be made: a property about how something feels at runtime
cannot be read out of source.

    git clone --depth 1 --branch v1.5.915 https://github.com/mumble-voip/mumble.git
    git -C mumble rev-parse HEAD
    5fe5ec6e61b0c1cc414a8a8db548ec484eec6b90

    git clone --depth 1 --branch v0.14.3 https://github.com/stoatchat/stoatchat.git
    git -C stoat rev-parse HEAD
    d5ca4a0fd6e6bdce0126e790a010fe0032d9b556

    git clone --depth 1 https://github.com/spacebarchat/server.git
    git -C spacebar rev-parse HEAD
    dcfd91035e3da42abf5f32d8d86a35219225b3d4
    git -C spacebar log -1 --format='%cI'
    2026-08-04T13:28:48+02:00

Spacebar publishes no release, so there is no version to name and the commit and
its date stand in for one:

    gh api repos/spacebarchat/server/releases/latest --jq '.tag_name'
    {"message":"Not Found", ... "status":"404"}

One name in issue #12 has moved. The project referred to there as the one whose
voice features spent a long time in beta is Revolt, and `revoltchat/backend` now
resolves to `stoatchat/stoatchat`:

    gh api repos/revoltchat/backend --jq '.full_name'
    stoatchat/stoatchat

It is surveyed under its current name throughout.

## The features

The five this project is betting on, in the wording of issue #12.

1. Switching between voice channels without a perceptible gap.
2. Rooms that exist with nobody in them.
3. A participant list visible without joining.
4. Per-person volume.
5. A documented bot interface with voice.

## Mumble, v1.5.915, commit 5fe5ec6

**Switching without a perceptible gap.** Partly observed, and the part that
matters is not measured. In source, a channel change is
`Server::userEnterChannel` in `src/murmur/Server.cpp:1967`, which moves the user
between channel objects and broadcasts a `UserState`. No transport is torn down
and no media session is renegotiated, because all audio for the connection rides
one tunnel regardless of channel. So the mechanism carries no renegotiation cost.
Whether the resulting gap is perceptible was NOT CHECKED, because that needs a
running instance and a measurement, and this survey has neither.

**Rooms that exist with nobody in them.** Observed. Channels are server-side
objects with their own persistence path, `Server::updateChannel` at
`src/murmur/ServerDB.cpp:1885` and `Server::removeChannelDB` at
`src/murmur/ServerDB.cpp:1872`, and they carry ACLs and groups independently of
anybody being present.

**Participant list visible without joining.** Observed. On authentication the
server walks the channel tree from the root and sends a `ChannelState` for each
node (`src/murmur/Messages.cpp:377`, the block commented `Transmit channel
tree`), then sends every authenticated user with the channel they are in:

    sed -n '476,502p' src/murmur/Messages.cpp
        // Transmit other users profiles
        foreach (ServerUser *u, qhUsers) {
        ...
                if (u->cChannel->iId != 0)
                        mpus.set_channel_id(u->cChannel->iId);

The first two lines and the last two are pasted; the twenty-three between them
set names, ids and textures and are elided.

A client therefore knows the occupancy of every channel it can see before it
enters any of them. This is the feature in its full form and not an
approximation of it.

**Per-person volume.** Observed. `ClientUser::setLocalVolumeAdjustment` in
`src/mumble/ClientUser.cpp:245` holds a per-user factor and
`src/mumble/AudioOutput.cpp:582` multiplies by it at mix time, so the gain is
applied in the client exactly as this project intends to apply it. The value is
client-local: it is not in the wire protocol as a per-speaker setting and does
not follow the person to another device. That last sentence is an observation
about `src/Mumble.proto` and `ClientUser`, not a test of a second device.

**Documented bot interface with voice.** Observed absent for voice, present for
everything else. The server's remote interface is `src/murmur/MumbleServer.ice`,
which exposes users, channels, ACLs, bans, text messages and callbacks. It
carries no audio surface at all:

    grep -in "audio\|voice\|opus\|pcm" src/murmur/MumbleServer.ice
    (no output)

So a bot that talks to the server's own interface cannot send or receive audio,
and a voice bot has to implement the client protocol itself. That is the
difference between a bot API with voice and a bot API beside one.

## Stoat, v0.14.3, commit d5ca4a0

**Switching without a perceptible gap.** Observed mechanism, gap NOT CHECKED.
Joining a voice channel is `POST /channels/<target>/join_call` in
`crates/delta/src/routes/channels/voice_join.rs`, which mints a LiveKit token and
returns a token and a server URL. Switching channel therefore costs an HTTP round
trip and a fresh media connection to whatever node the target channel is pinned
to, which is a heavier mechanism than Mumble's. Whether that is perceptible was
not measured here.

**Rooms that exist with nobody in them.** Observed. A voice channel is an
ordinary durable channel carrying an optional `VoiceInformation`
(`crates/core/models/src/v0/channels.rs:125`), read back through `Channel::voice`
at `crates/core/database/src/models/channels/model.rs:449`. The LiveKit room
behind it is created on demand, which is the same split this project chose.

**Participant list visible without joining.** Observed. The gateway `Ready` event
carries `voice_states: Option<Vec<ChannelVoiceState>>`
(`crates/core/database/src/events/client.rs:83`) and `ServerCreate` carries the
same list, and `ChannelVoiceState` holds `participants`
(`crates/core/models/src/v0/channels.rs:299`). A client learns who is in a voice
channel from the connection handshake, not from joining.

**Per-person volume.** Observed absent from the server. The whole Rust workspace
has no such setting:

    grep -rni "volume" --include=*.rs crates/
    (no output)

Whether the first-party client implements a local volume slider was NOT CHECKED.
That is a separate repository and this survey did not open it. Absent from the
server means only that it does not follow a person between devices.

**Documented bot interface with voice.** Partly observed. Bots are a first-class
account type and are not excluded from the voice join route; the only
bot-specific branch in it refuses one option:

    grep -n "bot" crates/delta/src/routes/channels/voice_join.rs
    39:    if user.bot.is_some() && force_disconnect == Some(true) {

So a bot can obtain a token and connect to the room like any other participant,
and voice is reachable through the same LiveKit client any participant uses.
Whether this is DOCUMENTED as a bot capability was NOT CHECKED: the route carries
an OpenAPI tag, which is API documentation, but the project's bot-facing
documentation lives in its documentation site and was not read.

## Spacebar, commit dcfd910, committed 2026-08-04

**The prior in issue #12 does not survive.** Issue #12 records that this project
has no voice or video support in any instance. At this commit the server contains
a complete voice signalling side: `src/webrtc/` with its own `Server`, `Identify`,
`SelectProtocol` and `Video` opcodes, a `VoiceStateUpdate` gateway opcode, and a
pluggable media backend loaded by name from the environment:

    sed -n '19,27p' src/webrtc/util/MediaServer.ts
    import type { SignalingDelegate } from "@spacebarchat/spacebar-webrtc-types";
    ...
    export const WRTC_PUBLIC_IP = process.env.WRTC_PUBLIC_IP ?? "127.0.0.1";

    grep -n "pion-webrtc\|webrtc-types" package.json
    48:        "@spacebarchat/spacebar-webrtc-types": "^1.0.1",
    81:        "@spacebarchat/pion-webrtc": "^0.0.4",

The media plane is not built in: `loadWebRtcLibrary` imports whatever
`WRTC_LIBRARY` names and fails with `NoConfiguredLibraryError` when nothing is
configured, so an instance that has not chosen a backend has no voice. That is a
different statement from the one in #12, and it is the accurate one at this
commit. Whether any public instance has configured a backend was NOT CHECKED,
because that is a claim about deployments and not about source.

**Switching without a perceptible gap.** NOT CHECKED. It depends on which media
backend an operator loaded and on whether a channel change reuses the voice
connection, and neither is decidable from the server repository alone.

**Rooms that exist with nobody in them.** Observed. A voice channel is
`GUILD_VOICE = 2` in `src/schemas/api/channels/Channel.ts:28`, a persisted
`Channel` row, durable in the same way every other channel is.

**Participant list visible without joining.** Observed. `VoiceState` is its own
persisted entity (`src/database/entities/VoiceState.ts`), a guild owns a
`voice_states` collection (`src/database/entities/Guild.ts:193`), and the guild
payload a client receives includes it:

    grep -n "voice_states" src/database/entities/Member.ts
    442:  voice_states: guild.voice_states.map((x) => x.toPublicVoiceState()),

**Per-person volume.** Observed absent from the server:

    grep -rni "user_volume\|userVolume" --include=*.ts src/
    (no output)

**Documented bot interface with voice.** NOT CHECKED. The project reimplements
another product's API, so what a bot may do is defined by that product's
documentation rather than by this repository, and judging it would mean reading
a specification this survey did not read.

## What a nearest project does better than this plan intends to

Mumble, in two places, and both are worth taking seriously rather than noting.

The first is the channel switch. A switch there is one `UserState` message and no
media renegotiation at all, because the audio path does not change when the
channel does. This project's own budget line in issue #6 allows p95 at or below
300 ms from click to first decoded frame of the new channel. Mumble's design
makes that interval a message round trip. Our number is a ceiling we have not yet
had to meet and Mumble's architecture is already comfortably underneath it. If
the LiveKit-backed design cannot get near that, the honest reading is that the
incumbent design was better on the one feature this product is most about, not
that 300 ms was always the target.

The second is listening to more than one channel at once. Mumble carries channel
listeners in its wire protocol with a per-listener, per-channel volume
adjustment, broadcast by the server:

    sed -n '183,187p' src/Mumble.proto
    message UserState {
            message VolumeAdjustment {
                    optional uint32 listening_channel = 1;
                    optional float volume_adjustment = 2;
            }

This project's plan has no equivalent. It also means Mumble already stores a
volume adjustment server-side, which is the half of per-person volume this
project treats as a synchronisation problem in
`docs/decisions/per-person-volume.md`, although Mumble does it per listened
channel rather than per speaker.

Neither of these is a reason to change the plan today. Both are reasons the
README's claim needs to be read narrowly.

## The README claim

The README says that the transport building blocks are open source and that what
this project builds is the orchestration, the client, and the finish that turns
those blocks into something a community would leave a polished commercial product
for.

The survey supports that claim in one narrow form only: no surveyed project
carries all five features. Mumble has three of them and no bot API with voice and
no video. Stoat has three and a per-speaker volume setting that exists nowhere in
its server. Spacebar has two and a media plane an operator has to supply.

The survey does NOT support any wider reading, and two readings that the sentence
invites are contradicted above. It does not support the idea that the open
alternatives are unpolished on these features, because the participant list
visible without joining is present in all three. It does not support the idea
that this project's targets are ahead of them, because Mumble's channel switch is
architecturally cheaper than this project's own budget line permits.

The README was left unchanged. Its sentence is about what this project builds and
does not assert either of the two readings the survey contradicts, so there is
nothing in it that this record makes false. The narrowing is recorded here, where
a reader who wants the evidence will find it, rather than by rewriting a sentence
the evidence does not refute.

## What this survey did not do

No running instance of any of the three was started. Every cell above that
depends on runtime behaviour says so. First-party clients were not opened, so
every client-side judgement is limited to what the server repository reveals. A
survey that started instances would decide the cells marked NOT CHECKED, and
nothing here should be read as having decided them.
