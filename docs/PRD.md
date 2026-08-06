# LOADIFY — Product Requirements Document

**Version:** 1.0
**Status:** Approved for build
**Owner:** Product / Architecture
**Market:** Uzbekistan (launch) → Central Asia → Europe → USA

---

## 1. Problem

Freight in Uzbekistan is brokered over Telegram groups and phone calls.

| Pain | Who feels it | Consequence |
|---|---|---|
| Loads posted as unstructured text in Telegram | Drivers | No filtering by truck type, weight, route → hours of scrolling |
| No verified identity | Both sides | Fraud, no-shows, cargo theft |
| No price transparency | Drivers | Undercut rates, no market benchmark |
| Empty return trips (deadhead) | Drivers | 30–50% of km driven unpaid |
| No visibility after pickup | Companies | Constant "where are you?" phone calls |
| Cash-only settlement | Both sides | No escrow, disputes unresolvable |

**Loadify replaces the Telegram group with a structured, verified, real-time marketplace.**

---

## 2. Goals

### Business goals
| # | Goal | Metric | Target (12 months) |
|---|---|---|---|
| G1 | Liquidity on both sides | Verified drivers / active companies | 10,000 / 1,200 |
| G2 | Marketplace throughput | Loads booked per month | 25,000 |
| G3 | Revenue | MRR from subscriptions + commission | $120k |
| G4 | Trust | Disputed trips as % of completed | < 1.5% |
| G5 | Retention | 90-day driver retention | > 55% |

### Product goals
- A driver finds and reserves a relevant load in **under 90 seconds**.
- A company publishes a load and receives the **first driver response in under 5 minutes**.
- Live location of an in-transit shipment is never more than **30 seconds stale**.

### Non-goals (v1)
- Loadify does not own trucks, employ drivers, or take custody of cargo.
- No customs brokerage, no freight forwarding licences, no insurance underwriting.
- No cross-border loads until Phase 3.

---

## 3. Personas

**Anvar — Owner-operator driver.** 34, one Isuzu Forward 5t, Android phone, mobile data only, Uzbek-speaking. Drives Tashkent↔Samarkand. Needs: a return load before he starts the outbound trip; a payer who won't disappear.

**Dilnoza — Logistics dispatcher.** 27, works at a 40-truck company, on a desktop 9 hours a day. Posts 15–30 loads a day. Needs: bulk posting, a live map of her assigned drivers, and one screen where she can see every trip's status.

**Rustam — Logistics company owner.** 45, runs the company, checks the app twice a day on his phone. Needs: analytics, cost per km, which dispatcher is performing, wallet balance.

**Admin — Loadify operations.** Verifies driver documents, resolves disputes, blocks fraud, monitors the marketplace.

---

## 4. Roles & permissions matrix

| Capability | Guest | Driver | Dispatcher | Company Manager | Admin | Super Admin |
|---|:-:|:-:|:-:|:-:|:-:|:-:|
| Browse public load list (masked contacts) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Register / verify phone | ✅ | — | — | — | — | — |
| Upload driver documents | — | ✅ | — | — | — | — |
| Set online/offline/busy | — | ✅ | — | — | — | — |
| Reserve a load | — | ✅ | — | — | — | — |
| Create / publish a load | — | — | ✅ | ✅ | ✅ | ✅ |
| View nearby drivers on live map | — | — | ✅ | ✅ | ✅ | ✅ |
| Book a driver directly | — | — | ✅ | ✅ | ✅ | ✅ |
| Assign a driver to a company load | — | — | ✅ | ✅ | ✅ | ✅ |
| Manage company dispatchers | — | — | — | ✅ | ✅ | ✅ |
| View company wallet & invoices | — | — | ❌ | ✅ | ✅ | ✅ |
| Company analytics | — | — | ✅ (read) | ✅ | ✅ | ✅ |
| Verify driver documents | — | — | — | — | ✅ | ✅ |
| Block / unblock users | — | — | — | — | ✅ | ✅ |
| Resolve complaints & disputes | — | — | — | — | ✅ | ✅ |
| Issue refunds | — | — | — | — | ✅ | ✅ |
| Manage roles & permissions | — | — | — | — | ❌ | ✅ |
| System settings, feature flags | — | — | — | — | ❌ | ✅ |
| Read audit logs | — | — | — | — | ✅ (own scope) | ✅ (all) |

Permissions are stored in the database (`permissions`, `role_permissions`) and enforced by the `RequirePermission` middleware — **never hardcoded in handlers**.

---

## 5. Functional requirements

### 5.1 Authentication & identity
| ID | Requirement | Priority |
|---|---|---|
| A1 | Register with phone number (+998 format), password, and role selection (driver / company) | P0 |
| A2 | 6-digit SMS OTP verifies the phone; TTL 5 min, max 5 attempts, 60s resend cooldown | P0 |
| A3 | Login by phone + password returns access (15 min) + refresh (30 days) tokens | P0 |
| A4 | Refresh rotation: using a refresh token invalidates it and issues a new pair | P0 |
| A5 | Reuse of an already-rotated refresh token revokes the entire token family (theft detection) | P0 |
| A6 | Access tokens are revocable via a Redis JTI denylist on logout / block | P0 |
| A7 | 5 failed logins lock the account for 15 minutes | P0 |
| A8 | Forgot password via OTP → reset token → new password | P0 |
| A9 | Multi-device sessions listed and individually revocable | P1 |
| A10 | Every session records device, platform, IP, user-agent, last-seen | P1 |

### 5.2 Driver
| ID | Requirement | Priority |
|---|---|---|
| D1 | Profile: name, photo, birth date, languages, home city, years of experience | P0 |
| D2 | Documents: driving licence (front/back), passport/ID, insurance, truck tech passport — each with expiry date | P0 |
| D3 | Document verification states: `pending` → `approved` / `rejected` (with reason) | P0 |
| D4 | Vehicle record: type, make, model, year, plate, photos, capacity (t), volume (m³), body dimensions L×W×H, fuel type, ADR flag, refrigerated flag, tail-lift flag | P0 |
| D5 | Trailer record linked to a vehicle (optional) | P1 |
| D6 | Status toggle: online / offline / busy — busy is set automatically on trip start | P0 |
| D7 | Publish availability: current location, free-from date, preferred destination regions, min acceptable price | P0 |
| D8 | Load feed with filters: pickup region, delivery region, weight, volume, truck type, price range, load date, distance from me | P0 |
| D9 | Map search with clustered load pins and a radius selector | P0 |
| D10 | Reserve a load — creates a `booking` in `pending` state and notifies the company | P0 |
| D11 | Contact reveal: phone number of the company is revealed only after reservation or with an active subscription | P0 |
| D12 | In-app chat per booking | P0 |
| D13 | Push notification when a load matching the driver's saved filters is published | P0 |
| D14 | Trip flow: accepted → en route to pickup → loaded → in transit → delivered | P0 |
| D15 | Navigation deep-link to Google Maps / Yandex Navigator | P1 |
| D16 | Trip history with earnings summary | P0 |
| D17 | Rate the company after trip completion (1–5 + comment) | P0 |
| D18 | Wallet: balance, transactions, payout requests | P1 |
| D19 | Premium subscription: unlimited contact reveals, priority in matching, early access to new loads | P1 |

### 5.3 Logistics company
| ID | Requirement | Priority |
|---|---|---|
| C1 | Company profile: legal name, brand name, INN/STIR, address, logo, contact persons, verification documents | P0 |
| C2 | Create load: cargo type, weight, volume, dimensions, pickup + delivery address with map pin, load date/window, price + currency + payment terms, required truck type, photos, documents, special requirements (ADR, temperature, tail-lift) | P0 |
| C3 | Save load as draft, publish, unpublish, duplicate | P0 |
| C4 | Load states: `draft` → `published` → `reserved` → `assigned` → `in_transit` → `delivered` → `completed`; plus `cancelled` and `expired` | P0 |
| C5 | Nearby drivers list + live map, filtered by truck type, capacity, radius, rating | P0 |
| C6 | Direct booking: offer a load to a specific driver | P0 |
| C7 | Accept / reject driver reservations | P0 |
| C8 | Cancel booking with a reason; cancellation policy affects rating | P0 |
| C9 | Live tracking of every in-transit shipment | P0 |
| C10 | Shipment history + export to CSV/XLSX | P1 |
| C11 | Company wallet, invoices, commission statements | P1 |
| C12 | Analytics: loads posted, fill rate, average time-to-fill, cost per km, spend by route, driver performance | P1 |
| C13 | Manage dispatchers: invite, assign permissions, deactivate | P0 |
| C14 | Rate the driver after completion | P0 |
| C15 | Premium subscription: more concurrent published loads, featured placement, advanced analytics | P1 |

### 5.4 Dispatcher
| ID | Requirement | Priority |
|---|---|---|
| P1 | Sees only the loads and drivers of their own company | P0 |
| P2 | Create and publish loads on behalf of the company | P0 |
| P3 | Assign drivers to loads | P0 |
| P4 | Track all active trips on one board | P0 |
| P5 | Upload and manage shipment documents (CMR, invoice, waybill) | P1 |
| P6 | Monitor which company drivers are online | P0 |

### 5.5 Admin panel
Dashboard (live KPIs) · Users · Drivers (+ document verification queue) · Companies (+ verification queue) · Loads · Trips · Payments · Subscriptions · Complaints · Reports · Analytics · Notification broadcasts · Roles · Permissions · System settings · Audit logs · CMS (static pages, FAQ, banners) · Support inbox.

### 5.6 Realtime
| ID | Requirement | Latency budget |
|---|---|---|
| R1 | Driver publishes location every 15s while on trip, every 60s while idle-online | ≤ 30s staleness |
| R2 | Company map receives driver position updates over WebSocket | ≤ 2s from ingest |
| R3 | Booking created / accepted / rejected pushed to both parties | ≤ 1s |
| R4 | Chat message delivery | ≤ 500ms |
| R5 | New matching load pushed to subscribed drivers | ≤ 5s from publish |

### 5.7 AI features (Phase 2)
| Feature | Approach v1 (ships with Phase 2) | Upgrade path |
|---|---|---|
| Best-driver recommendation | Weighted score: distance to pickup, truck fit, rating, completion rate, response time, price history | Learning-to-rank on booking outcomes |
| Best-load recommendation | Same scorer inverted + deadhead minimisation against the driver's declared destination | Gradient-boosted ranker |
| Price prediction | Median of last 90 days for (origin region, destination region, truck type, weight bucket), adjusted for fuel index and seasonality | Gradient boosting on trip features |
| ETA prediction | Google Distance Matrix with traffic + historical correction factor per corridor | Model on our own GPS traces |
| Fraud detection | Rules: new account + high-value load + off-platform contact push + mismatched documents + velocity anomalies | Anomaly detection model |
| Duplicate load detection | Normalised hash of (company, route, weight, load date) + text similarity on description | Embedding similarity |
| Route optimisation | Google Directions with waypoint ordering | Custom VRP solver for multi-drop |
| Demand forecasting | Time series per corridor, weekly seasonality | Prophet / LSTM |
| Driver & company scoring | Composite: completion rate, on-time %, cancellation rate, rating, dispute rate, document validity | Behavioural model |

Every AI feature sits behind a `ScoringService` interface so the rules implementation can be swapped for a model without touching callers.

### 5.8 Payments
| ID | Requirement | Phase |
|---|---|---|
| M1 | Subscription plans (driver Basic/Premium, company Start/Business/Enterprise) with monthly + yearly pricing | 1 |
| M2 | Click, Payme, Uzum Bank checkout + webhook confirmation | 2 |
| M3 | Wallet with double-entry ledger — every movement is two rows, balance is derived | 2 |
| M4 | Escrow: company funds held on booking, released on delivery confirmation, refundable on dispute | 2 |
| M5 | Platform commission (configurable %, default 5%) deducted on release | 2 |
| M6 | Invoices (PDF) and transaction statements | 2 |
| M7 | Refunds with admin approval and full audit trail | 2 |
| M8 | Idempotency keys on every payment write; webhooks are replay-safe | 2 |

---

## 6. Non-functional requirements

| Category | Requirement |
|---|---|
| Availability | 99.9% monthly for the API |
| Latency | p95 < 300ms for reads, < 600ms for writes (excluding third-party calls) |
| Throughput | 2,000 req/s sustained; 50,000 concurrent WebSocket connections per node |
| Scale targets | 500k users, 5M loads, 20M location points/day at 24 months |
| Data retention | Location traces 90 days hot / 1 year cold; audit logs 5 years |
| Localisation | Uzbek (Latin), Russian, English — all user-facing strings externalised |
| Currency | UZS primary, USD secondary; amounts stored as `BIGINT` minor units |
| Timezone | All timestamps `TIMESTAMPTZ` in UTC; rendered in Asia/Tashkent |
| Accessibility | WCAG 2.1 AA on web |
| Mobile | Android 8+ / iOS 13+; must function on 3G |
| Compliance | Uzbek Personal Data Law No. ZRU-547 — personal data stored in-country |
| Backups | Postgres PITR, daily base backup, 30-day retention, restore drill quarterly |

---

## 7. Success metrics

**North star:** *booked loads per week.*

| Funnel stage | Metric |
|---|---|
| Acquisition | Registrations by role and channel |
| Activation | % drivers reaching `verified` within 48h |
| Engagement | DAU/MAU; searches per driver per day |
| Marketplace | Fill rate, median time-to-first-response, median time-to-fill |
| Monetisation | Paid conversion %, ARPU, commission GMV |
| Retention | D1/D7/D30/D90 by cohort |
| Quality | Cancellation rate, dispute rate, average rating |

---

## 8. Risks & mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Cold start — no drivers, so no companies | Fatal | Seed one corridor (Tashkent↔Samarkand) first; onboard 300 drivers manually with free premium |
| Disintermediation (parties go off-platform after first contact) | High | Value stays in-platform: escrow, ratings, dispute resolution, document storage, subscription instead of per-deal fees |
| Fraud / fake cargo | High | Mandatory document verification, company INN check, escrow, rate-limited contact reveals, fraud scoring |
| GPS battery drain kills adoption | Medium | Adaptive sampling: 15s in-trip, 60s idle, geofence-triggered wake, stop tracking when offline |
| Payment provider integration delays | Medium | Phase 1 launches with subscriptions only; escrow is Phase 2 |
| Poor connectivity on rural routes | Medium | Offline queue in the Flutter app, replay on reconnect, WS auto-reconnect with exponential backoff |

---

## 9. Roadmap

### Phase 1 — MVP (marketplace core)
Authentication · Profiles (driver, company, dispatcher) · Documents & verification · Vehicles · Load posting · Load & driver search · Maps · Bookings · Trips · Realtime tracking · Chat · Notifications · Subscriptions · Admin panel · Ratings

### Phase 2 — Money & intelligence
Click / Payme / Uzum · Wallet & double-entry ledger · Escrow & commission · Invoices & refunds · Analytics & reports · AI matching, price and ETA prediction · Fraud detection

### Phase 3 — International
Multi-country regions & address model · Cross-border documents (CMR, TIR, customs) · Multi-currency settlement · Stripe · Carrier/forwarder role · EU & US market adaptation

---

## 10. Release criteria (Phase 1)

- [ ] All P0 requirements implemented and covered by automated tests
- [ ] Backend test coverage ≥ 70% on usecase and repository layers
- [ ] Load test: 2,000 req/s sustained for 10 minutes, p95 < 300ms
- [ ] Security review passed: OWASP API Top 10, dependency scan clean of high/critical
- [ ] Swagger spec complete and matching the implementation
- [ ] Docker Compose stack starts from scratch with one command
- [ ] Deployment runbook validated on a staging DigitalOcean droplet
- [ ] uz / ru / en translations complete on web and mobile
