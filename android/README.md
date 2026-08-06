# Loadify — driver app

The mobile half of Loadify. Companies post and manage freight from the web
client; drivers work from here, which is why this app is built around four
things a driver does in a cab: find a load, book it, run the trip, get paid.

## Running it

The app talks to the Go API in `../backend`. Start that first, then:

```bash
flutter run --dart-define=LOADIFY_API_URL=http://10.0.2.2:8080/api/v1
```

`10.0.2.2` is the Android emulator's alias for your machine. On a physical
device use your LAN address instead — `localhost` there is the phone.

Cleartext HTTP is permitted only for those loopback aliases
(`android/app/src/main/res/xml/network_security_config.xml`); everything else
must be HTTPS, so a release build cannot silently talk to a plain-HTTP host.

### Build flags

| Flag | Default | Purpose |
| --- | --- | --- |
| `LOADIFY_API_URL` | `http://10.0.2.2:8080/api/v1` | REST base URL |
| `LOADIFY_WS_URL` | `ws://10.0.2.2:8080/api/v1/ws` | Realtime hub |
| `LOADIFY_MAPS_KEY` | *(empty)* | Google Maps; absent means map views degrade to a coordinate readout rather than crashing |

Release:

```bash
flutter build apk --release --dart-define=LOADIFY_API_URL=https://api.loadify.uz/api/v1
```

## Layout

```
lib/
  app/          composition root — providers, router, theme, locale
  core/         config, errors, HTTP, storage, formatting, shared widgets
  features/
    auth/       login, session, current principal
    loads/      the board, filters, load detail
    bookings/   offers from both sides
    trips/      the state machine and location reporting
    driver/     profile, availability, vehicles, wallet
  l10n/         uz / ru / en
```

Each feature is `domain` (entities, no Flutter imports) → `data` (repository
over the API client) → `presentation` (screens). Composition happens only in
`app/providers.dart`, so a feature never reaches for another feature's
repository directly.

## Decisions worth knowing

**Permissions, not roles.** The UI gates on `user.can('bookings.create')`, never
on a role name. Adding a role server-side would otherwise require an app release
— and on mobile that means waiting on a store review.

**One refresh at a time.** Concurrent 401s share a single refresh future. Without
that, four screens refreshing at once each send their own refresh; the backend
rotates the token on first use, so the rest present an already-rotated token and
trip its theft detection, signing the driver out for doing nothing wrong.
`test/api_client_test.dart` guards this.

**Tokens are nested.** `/auth/login` and `/auth/refresh` return
`data.tokens.access_token`, not `data.access_token`. Reading the wrong one
yields null, which is then sent as no header at all — the app looks broken
rather than signed out. Parsing is strict and a malformed refresh signs out
cleanly. The web client actually shipped this bug; the tests exist so this one
does not repeat it.

**Money is an integer.** Minor units (tiyin) with an explicit currency, never a
double. `Fmt.money` is the only thing that renders it.

**Foreground location only.** A trip reports position every 30s while it is
actually moving — not before pickup, not after delivery. Background location
needs a separate Play justification and prominent disclosure; that is a
deliberate later step, not an oversight.

**The board is public.** A driver evaluating the app sees real freight before
signing up, with contact details masked until a booking exists. Only the
authenticated tabs redirect to login.

**Reference data is translated server-side.** Region and vehicle-type names come
back already localised for the `Accept-Language` header, so switching language
re-fetches them rather than translating twice and drifting.

## Tests

```bash
flutter analyze
flutter test
```

Covers the API client's refresh behaviour, auth payload parsing, the trip state
machine, booking permissions, filter semantics, and login validation. It
deliberately does not mock the whole app — the value is at the contract edges,
where a wrong assumption is silent.

## Not yet built

- Push notifications (FCM). The backend registers devices at
  `POST /notifications/devices`; the client side is not wired up.
- Chat. The rooms and messages endpoints exist and are used by the web client.
- In-app map rendering. Positions are reported; drawing them needs a Maps key.
- Registration and OTP. Drivers are onboarded server-side for now, so the app
  signs in only.
