# What a public funding application would require

Written 2026-08-06. Raised by issue #86.

This document says what applying would cost and what is already in place. It does
not say whether to apply. That is entry 4 of issue #1 and belongs to the
maintainer.

## What was open on the day this was written

The funder is NLnet. The route this project would obviously have taken is closed:

    curl -s https://nlnet.nl/commonsfund/ | sed -e 's/<[^>]*>/ /g' | tr -s ' ' ' ' | grep -i 'final call'
     The thirteenth and final call of NGI Zero Commons Fund closed on June 1 st 2026. No more new aplications are sought for this fund, but we have other programmes running. Check out the Apply for funding page.

The output above is the page's own sentence with its HTML tags stripped, which is
why the ordinal reads as `June 1 st 2026` and why one word is misspelled. Neither
is an error in this document.

Two programmes were open:

    curl -s https://nlnet.nl/funding.html | sed -e 's/<[^>]*>/ /g' | tr -s ' \n' ' \n' | sed -n '111,121p'
     Active funds
     New funds will become active after the summer. We're currently transitioning from our Next Generation Internet programmes to the Open Internet Stack. The upcoming funds are announced on our homepage .

     Open Social Fund
     Promoting W3C ActivityPub and beyond
     The goal of the Open Social Fund is to help to restore balance in the social media landscape of today and tomorrow. [...]

     Research and Higher Education Technology Fund
     Today we make the internet of tomorrow
     Financial cuts and the 'publish or perish' paradigm are increasingly impacting the capability of the research and education community to contribute to the future of the internet [...]

The two fund descriptions are elided at `[...]`; nothing else is cut. That
command selects by line number after tag stripping, so the range moves when the
page changes. The re-check section below gives a form that does not.

Three more are named as upcoming rather than open, in the same page's `Upcoming
funds` block: Restack, CodeSupply and ELFA. The funding page gives no scope for
any of them, but the funder's homepage does:

    curl -s https://nlnet.nl/ | sed -e 's/<[^>]*>/ /g' | grep -i -n 'Restack\|CodeSupply\|ELFA'
    124:  The goal of  Restack  is to build a healthy Open Internet Stack. The fund provides practical and financial support to projects contributing to an open, resilient and trustworthy digital infrastructure, paving the way to permissionless innovation and user autonomy.
    129:   CodeSupply  is a  scale-up programme  which addresses data challenges in cybersecurity, software supply chain management, and regulatory and open source license compliance. Funding is available for FOSS efforts that align with the programme's three main objectives.
    137:   ELFA  (Encrypted Local First Architecture) is a  scale-up programme  which aims to create a full-fledged collaborative and decentralized open platform capable of providing private workspaces and healthy social networking. This includes an integrated suite of collaborative apps with end-to-end encryption.

Each of the three is marked `Coming soon` on that page and none of them carries a
date.

Restack is the one to watch. Its stated goal, open and resilient digital
infrastructure and user autonomy, is a description a self-hosted voice and video
service fits without any stretching. That is a reading of a two-sentence
description and not an eligibility check, and no eligibility text exists yet to
check against.

This funder has also paid for work of exactly this kind before. Its own homepage
lists vendor-independent videoconferencing among the things it has funded, and
Jitsi among the projects it has supported. That is a reason to think the fit is
real when a fitting programme opens, and it is not a reason to think an
application would succeed.

## The finding

Neither open programme fits a self-hosted voice and video service.

The Open Social Fund is for the social media landscape and names W3C ActivityPub
as its subject. This project's federation question is issue #7 and is not
answered, and even a federating version of it would be a real-time media service
rather than a social publishing one.

The Research and Higher Education Technology Fund is for research and education
networks and their users and providers. This project has no research or education
network in it.

So the honest answer today is that the fitting route is not open. That is worth
more than it sounds: it is the difference between knowing this now and finding it
out three weeks into writing an application.

## What an application requires

Common to every programme this funder runs, taken from the page quoted above.

A proposal in English, kept short, with a research and development focus and a
European dimension.

A work plan divided into tasks, with a budget per task. Grants run from 5,000 to
50,000 euro, scalable where potential is proven, on a cost-recovery basis rather
than at commercial rates.

Every output under a recognised free and open source licence, in its entirety,
with scientific results published open access.

A deadline on the third of every odd month, with back-to-back calls, so there is
always a next one.

Scoring is on technical merit, relevance to the fund, and value for money.

## What this project already has, and what it would still have to produce

Already in place.

A recognised free and open source licence, in the tree:

    git ls-files LICENSE
    LICENSE
    gh api repos/iderex/stammtisch --jq '.license.spdx_id'
    AGPL-3.0

A milestone structure with deliverables. The board is already divided into
milestones whose issues carry a done-condition each, which is most of the shape a
work plan wants. It is not a work plan yet, because it has no task budget and no
time estimate attached to anything.

A public repository with its reasoning written down. The decision records under
`docs/decisions/` are the kind of evidence a technical merit score is made
against.

Still to produce, and none of it exists today.

A budget per task, in euro, which means somebody has to estimate effort per issue.

A European dimension stated explicitly rather than left implicit in where the
work happens.

A statement of what the research and development content is, as distinct from
engineering. This is the part most likely to be the weak one, because most of
this board is careful engineering rather than research, and a proposal that
claims otherwise will be read by people who can tell.

## The precondition this issue named

Issue #86 was written expecting the licence to be the unmet precondition, and
names entry 1 of issue #1 as the entry that holds it.

That entry has been answered since. The licence is AGPL-3.0 and the file is in
the tree, shown by the two commands above, and it is a recognised free and open
source licence, so it satisfies the requirement every one of this funder's
programmes places on outputs.

So the precondition this issue names is met. Saying it is unmet because the issue
expected it to be would be false, and reporting it as met is not a softening of
anything: it is one requirement satisfied out of a list that still has unfitting
programmes at the top of it.

What is unmet is different in kind. No open programme fits, which is the finding
above, and whether to apply at all is entry 4 of issue #1 and is not decided here.

## How to re-check

Do not repeat the research. Run these.

Which programmes are open, by name, without depending on a line number:

    curl -s https://nlnet.nl/funding.html | sed -e 's/<[^>]*>/ /g' | grep -A2 -i 'Active funds'

Whether the closed fund has reopened or been replaced:

    curl -s https://nlnet.nl/commonsfund/ | sed -e 's/<[^>]*>/ /g' | grep -i 'call'

Whether any of the three upcoming funds has opened. They are announced on the
funder's homepage rather than on the funding page, so that is where to look:

    curl -s https://nlnet.nl/ | sed -e 's/<[^>]*>/ /g' | grep -i -n 'Restack\|CodeSupply\|ELFA'

The thing worth watching is Restack, for the reason given above. It is the
transition the funding page names, from the Next Generation Internet programmes
to the Open Internet Stack, arriving as a fund.

Re-check when one of the three opens, or at the next odd-month deadline,
whichever comes first. Both of the above are commands rather than a memory.

## What this document does not do

It does not evaluate any funder other than NLnet. Public funding for this kind of
work exists elsewhere in Europe, and no command here looked at any of it, so
nothing in this document should be read as a survey of the field.

It does not estimate the preparation effort in hours or the reporting burden of
an award, because both were named in issue #1 as costs and neither was measured
here.
