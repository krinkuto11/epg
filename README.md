# epg

An XMLTV guide for Spanish TV whose live markers are right.

It is [dobleM's guide](https://github.com/davidmuma/EPG_dobleM) (`guiatv.xml`) with the live markers taken from
Movistar Plus+'s own guide. Everything else passes through unchanged.

```
https://raw.githubusercontent.com/krinkuto11/epg/dist/guiatv.xml
https://raw.githubusercontent.com/krinkuto11/epg/dist/guiatv.xml.gz
```

`status.json` next to it says when the guide was built, from which dobleM build, and what was changed.

## Why

dobleM marks a live programme by starting its title with `DIRECTO`, but the marker often lands on the wrong airing.
On 14-16 October 2026, across 34 sports channels:

- it was **missing on 12 live events**, among them Roma - Real Madrid and Betis - Oporto (Champions League) and four
  Euroleague games;
- it was **present on 11 programmes that are not live**, all of them pre-match shows.

A live match and its replays have the same title and description, so a guide that only has titles cannot tell them
apart. Movistar's JSON guide flags each airing itself (`Directo`), and that flag was right on every match.

## What changes

For each programme on a channel Movistar carries (158 of the guide's 640), the Movistar airing that starts at the
same minute decides:

- the title starts with `DIRECTO ` exactly when the airing is live;
- the programme gets `<live/>` when it is live and `<premiere/>` when it is a first showing. Dispatcharr stores
  these and returns them as `is_live` and `is_premiere`.

A programme with no Movistar airing at the same start time is left untouched, and so are the days beyond the three
that are read (today and the next two). If Movistar cannot be read, the guide is dobleM's unchanged, and
`status.json` says why.

Channels are paired by name: the guide's channel id or any display name, ignoring case and repeated spaces, against
the names on Movistar's TV-guide page. To force or prevent a pairing, add `channels.json` to the repo, mapping guide
channel ids to Movistar codes (`""` = never touch the channel):

```json
{"M+ Liga de Campeones HD": "CHAPIO", "Some Channel": ""}
```

## How it runs

The `build` workflow runs every hour. It builds the guide (about 15 s, roughly 470 small requests to Movistar) and,
when the guide changed, replaces the single commit on the `dist` branch.

```sh
go run ./cmd/epg -out dist                 # build locally
go run ./cmd/epg -base guiatv.xml -out /tmp/epg   # from a local copy of the base guide
```
