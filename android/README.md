# Loadify — driver app

Native Android. Companies post and manage freight from the web client; drivers
work from here, so the app is built around the four things a driver does in a
cab: find a load, book it, run the trip, get paid.

Kotlin, Jetpack Compose, Material 3, Hilt, Retrofit/OkHttp, kotlinx.serialization.

## Running it

The app talks to the Go API in `../backend`. Start that first, then open this
directory in Android Studio and run, or:

```bash
./gradlew :app:installDebug
```

The debug build defaults to `http://10.0.2.2:8080/api/v1` — the emulator's alias
for your machine. On a physical device pass your LAN address, because
`localhost` there is the phone:

```bash
./gradlew :app:installDebug -PloadifyApiUrl=http://192.168.1.20:8080/api/v1
```

Cleartext HTTP is permitted only for those loopback aliases
(`app/src/main/res/xml/network_security_config.xml`); everything else must be
HTTPS, so a release build cannot silently talk to a plain-HTTP host.

### Build properties

| Property | Debug default | Release default |
| --- | --- | --- |
| `loadifyApiUrl` | `http://10.0.2.2:8080/api/v1` | `https://api.loadify.uz/api/v1` |
| `loadifyWsUrl` | `ws://10.0.2.2:8080/api/v1/ws` | `wss://api.loadify.uz/api/v1/ws` |

```bash
./gradlew :app:assembleRelease -PloadifyApiUrl=https://api.loadify.uz/api/v1
```

The release build is unsigned; wire a keystore into `signingConfigs` before
shipping. `*.jks` and `*.keystore` are gitignored.

## Layout

```
app/src/main/java/uz/loadify/
  core/
    config/     build-time values
    di/         Hilt modules — the only place wiring happens
    error/      ApiException, SessionExpiredException
    format/     money, weight, dates, phone
    network/    Retrofit surface, envelope, interceptors, authenticator
    storage/    the session, in EncryptedSharedPreferences
    ui/         theme, shared composables, UiState
  feature/
    auth/       login, session, current principal
    loads/      the board, filters, load detail
    bookings/   offers from both sides
    trips/      the state machine and location reporting
    driver/     profile, availability, vehicles, wallet
```

Each feature is `domain` (entities, no Android imports) → `data` (repository
over the API) → `ui` (ViewModel + composables). Composition happens only in
`core/di`, so a feature never reaches for another feature's repository.

## Decisions worth knowing

**Permissions, not roles.** The UI gates on `user.can("bookings.create")`, never
on a role name. Adding a role server-side would otherwise require an app
release — and on mobile that means waiting on a store review.

**One refresh at a time.** `TokenAuthenticator` holds a lock and re-checks
whether another thread already rotated the token. Without that, four screens
hitting a 401 at once each send their own refresh; the backend rotates on first
use, so the rest present an already-rotated token and trip its theft detection,
signing the driver out for doing nothing wrong.

**Anonymous calls are tagged, not headered.** An OkHttp header set by the app
and stripped by the interceptor is invisible to the authenticator, which sees
the modified request — so an anonymous 401 would have been refreshed and
replayed *with* credentials. A tag never reaches the wire and survives to the
authenticator. `TokenAuthenticatorTest` is what caught this.

**Tokens are nested.** `/auth/login` and `/auth/refresh` return
`data.tokens.access_token`, not `data.access_token`. Reading the wrong one
yields an empty string, which is sent as no header at all — the app then looks
broken rather than signed out. `AuthResponse.isUsable` rejects the bad shape
outright, and a malformed refresh signs out cleanly. The web client shipped
exactly this bug; the tests exist so this one does not repeat it.

**Money is a Long.** Minor units (tiyin) with an explicit currency, never a
Double. `Fmt.money` is the only thing that renders it.

**Foreground location only.** A trip reports position every 30s while it is
actually moving — not before pickup, not after delivery. Background location
needs a separate Play justification and prominent disclosure; that is a
deliberate later step, not an oversight.

**The board is public.** A driver evaluating the app sees real freight before
signing up, with contact details masked until a booking exists. Only the other
three tabs redirect to login.

**Reference data is translated server-side.** Region and vehicle-type names come
back already localised for the `Accept-Language` header, so switching language
re-fetches them rather than translating twice and drifting.

**`minSdk 24` with core library desugaring.** `java.time` is API 26+, so without
desugaring the date formatters crash on Android 7 — a real slice of this
market's devices, and the users least likely to report it.

## Verify

```bash
./gradlew :app:assembleDebug :app:testDebugUnitTest :app:lintDebug :app:assembleRelease
```

Kotlin compiles with `allWarningsAsErrors`, so a deprecation fails the build
rather than accumulating. That is what caught the non-mirrored arrow icons that
would have pointed the wrong way in an RTL layout.

24 unit tests cover the authenticator's refresh behaviour under concurrency,
auth payload parsing, the trip state machine, booking permissions, filter
semantics, and int64 money values. They deliberately do not mock the whole app —
the value is at the contract edges, where a wrong assumption is silent.

The release task runs R8. Keeping it in the loop matters: minification strips
kotlinx.serialization's generated serializers unless `proguard-rules.pro` keeps
them, and that failure appears only in release.

## Not yet built

- Push notifications (FCM). The backend registers devices at
  `POST /notifications/devices`; the client side is not wired up.
- Chat. The rooms and messages endpoints exist and are used by the web client.
- In-app map rendering. Positions are reported; drawing them needs a Maps key.
- Registration and OTP. Drivers are onboarded server-side for now, so the app
  signs in only.
