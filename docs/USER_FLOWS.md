# LOADIFY — User Flows

All diagrams are Mermaid. States in `code` match the enum values in the database.

---

## 1. Driver registration & verification

```mermaid
flowchart TD
    A[Open app] --> B[Choose role: Driver]
    B --> C[Enter phone +998XXXXXXXXX]
    C --> D[POST /auth/register]
    D --> E[SMS OTP sent · TTL 5 min]
    E --> F{Enter 6-digit code}
    F -- wrong, attempts < 5 --> F
    F -- 5 wrong attempts --> G[OTP invalidated · request new code]
    G --> E
    F -- correct --> H[POST /auth/verify-otp<br/>phone_verified_at set]
    H --> I[Set password]
    I --> J[Tokens issued · status = pending_verification]
    J --> K[Fill driver profile]
    K --> L[Upload documents<br/>licence · passport · insurance]
    L --> M[Add vehicle + photos + dimensions]
    M --> N[Submit for verification]
    N --> O{Admin review}
    O -- reject --> P[Push: reason · fix and resubmit]
    P --> L
    O -- approve --> Q[status = verified · full access]
    Q --> R[Driver can go online and reserve loads]
```

**Before verification** a driver may browse loads with masked contact details but cannot reserve.

---

## 2. Company registration & verification

```mermaid
flowchart TD
    A[Choose role: Logistics company] --> B[Phone + OTP + password]
    B --> C[Company profile<br/>legal name · STIR/INN · address · logo]
    C --> D[Upload registration certificate + licence]
    D --> E[Owner becomes company_manager]
    E --> F{Admin review}
    F -- reject --> G[Reason · resubmit]
    G --> D
    F -- approve --> H[company.status = verified]
    H --> I[Can publish loads · invite dispatchers]
    I --> J[Invite dispatcher by phone]
    J --> K[Dispatcher registers via invite link<br/>role = dispatcher, scoped to company]
```

---

## 3. Load lifecycle (company side)

```mermaid
stateDiagram-v2
    [*] --> draft: dispatcher creates
    draft --> published: publish (validated + company verified)
    draft --> [*]: delete
    published --> reserved: driver reserves / company offers
    published --> expired: load_date passed
    published --> cancelled: company cancels
    reserved --> published: company rejects reservation
    reserved --> published: driver withdraws
    reserved --> assigned: company accepts → trip created
    assigned --> in_transit: driver marks loaded
    assigned --> cancelled: cancel with reason (penalty)
    in_transit --> delivered: driver marks delivered
    delivered --> completed: company confirms → escrow released, ratings open
    delivered --> disputed: company raises a dispute
    disputed --> completed: admin resolves in driver's favour
    disputed --> cancelled: admin resolves in company's favour → refund
    cancelled --> [*]
    completed --> [*]
    expired --> [*]
```

---

## 4. Driver finds and reserves a load

```mermaid
sequenceDiagram
    autonumber
    participant D as Driver app
    participant API as Loadify API
    participant WS as WebSocket hub
    participant C as Company web

    D->>API: PATCH /drivers/me/status {online}
    D->>API: GET /loads?pickup_region=..&truck_type=..&radius_km=100
    API-->>D: paginated loads + distance + match score
    D->>API: GET /loads/{id}
    API-->>D: full detail, company contact masked
    D->>API: POST /loads/{id}/reserve
    API->>API: validate driver verified · vehicle fits · no conflicting trip
    API->>API: create booking {status: pending}
    API->>WS: publish booking.created → company channel
    WS-->>C: toast + badge, live
    API-->>D: booking pending · company contact revealed
    C->>API: POST /bookings/{id}/accept
    API->>API: booking accepted · load.assigned · trip created
    API->>WS: publish booking.accepted → driver channel
    WS-->>D: push + in-app notification
    D->>API: GET /chats/{booking_id}/messages
```

Reservation guards, all enforced server-side:
- driver `status = verified` and has at least one approved vehicle
- vehicle capacity ≥ load weight **and** vehicle volume ≥ load volume
- vehicle body type matches `required_truck_type`
- driver has no other trip overlapping the load window
- driver has not exceeded the free contact-reveal quota (unless premium)

---

## 5. Trip execution & live tracking

```mermaid
sequenceDiagram
    autonumber
    participant D as Driver app
    participant API as API
    participant R as Redis pub/sub
    participant WS as WS hub
    participant C as Company web

    D->>API: POST /trips/{id}/start
    API->>API: trip.status = en_route_to_pickup · driver.status = busy
    loop every 15 s while on trip
        D->>WS: {lat, lng, speed, heading, accuracy, recorded_at}
        WS->>R: PUBLISH driver:location
        WS->>API: buffered batch insert into driver_locations
        R-->>WS: fan-out to subscribers
        WS-->>C: live marker update
    end
    D->>API: POST /trips/{id}/arrived-pickup
    D->>API: POST /trips/{id}/loaded (photos + waybill)
    API->>API: trip.status = in_transit · load.in_transit
    D->>API: POST /trips/{id}/arrived-delivery
    D->>API: POST /trips/{id}/delivered (POD photos + signature)
    API->>API: trip.status = delivered
    API->>WS: shipment.delivered → company
    C->>API: POST /trips/{id}/confirm
    API->>API: trip.completed · escrow released · commission taken
    API->>API: driver.status = online · ratings unlocked for both
```

Offline handling: the Flutter app queues location points in local SQLite and flushes them on reconnect; the server accepts back-dated points and orders by `recorded_at`.

---

## 6. Company books a specific driver

```mermaid
flowchart TD
    A[Company: live driver map] --> B[Filter: truck type · capacity · radius · min rating]
    B --> C[Select driver]
    C --> D[View profile · vehicle · rating · completed trips]
    D --> E[POST /drivers/{id}/offer with load_id + price]
    E --> F[booking {status: offered} · push to driver]
    F --> G{Driver responds within 30 min}
    G -- accept --> H[booking accepted · trip created]
    G -- reject --> I[Company notified · offer another driver]
    G -- timeout --> J[booking expired · auto-suggest next best driver]
```

---

## 7. Chat

```mermaid
sequenceDiagram
    participant D as Driver
    participant WS as WS hub
    participant API as API
    participant C as Company

    D->>API: GET /chats?booking_id={id}
    API-->>D: chat room (created with the booking)
    D->>WS: connect /ws?token=JWT
    D->>WS: {type: chat.send, room_id, body}
    WS->>API: persist message
    WS-->>C: chat.message event
    Note over C: if offline → FCM push
    C->>WS: {type: chat.read, room_id, up_to_message_id}
    WS-->>D: read receipt
```

Chat rooms exist only for a booking and are archived (read-only) 30 days after trip completion.

---

## 8. Subscription purchase

```mermaid
flowchart TD
    A[Open Premium screen] --> B[Choose plan + billing period]
    B --> C[POST /subscriptions/checkout]
    C --> D[Order created · status pending · idempotency key]
    D --> E[Redirect to Click / Payme / Uzum]
    E --> F{Provider result}
    F -- success --> G[Webhook → verify signature → mark order paid]
    G --> H[Subscription activated · expires_at set]
    H --> I[Entitlements refreshed · push confirmation]
    F -- failure --> J[Order failed · user may retry]
    F -- no webhook in 15 min --> K[Reconciliation job polls provider status]
    K --> G
```

---

## 9. Dispute

```mermaid
flowchart TD
    A[Trip delivered] --> B{Company confirms within 72h?}
    B -- yes --> C[Completed · escrow released]
    B -- no --> D[Auto-confirm · escrow released]
    B -- raises dispute --> E[complaint created · escrow frozen]
    E --> F[Both parties upload evidence]
    F --> G[Admin reviews]
    G -- driver at fault --> H[Partial or full refund to company]
    G -- company at fault --> I[Full release to driver]
    G -- shared --> J[Split per admin decision]
    H & I & J --> K[Resolution logged in audit_logs · both parties notified]
```

---

## 10. Admin document verification

```mermaid
flowchart TD
    A[Admin dashboard] --> B[Verification queue sorted by wait time]
    B --> C[Open submission: documents + OCR hints + fraud score]
    C --> D{Decision}
    D -- approve --> E[document.status = approved]
    E --> F{All required documents approved?}
    F -- yes --> G[driver.status = verified · welcome push]
    F -- no --> B
    D -- reject --> H[Select reason + free text]
    H --> I[document.status = rejected · push with reason]
    I --> B
    D -- request more info --> J[Message the driver via support thread]
    C --> K[Every action written to audit_logs]
```

---

## 11. Guest → registered conversion

```mermaid
flowchart LR
    A[Guest opens web] --> B[Browse loads · contacts masked]
    B --> C{Taps 'Show contact' or 'Reserve'}
    C --> D[Auth wall with the load kept in context]
    D --> E[Register → OTP → verify]
    E --> F[Returns to the same load, action resumed]
```
