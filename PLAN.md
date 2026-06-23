# Implementation Plan — ShortLink

> Dựa trên [requirement.md](requirement.md) (yêu cầu + quyết định đã chốt) và [RULES.md](RULES.md) (quy ước).

## Trạng thái hiện tại

Đã lỡ tạo code dùng **BoltDB + hash 6-ký-tự**. Quyết định đã chốt là **SQLite + counter + Feistel obfuscation → base62(6)**. → Phải viết lại `store`, `shortener`, và cập nhật handler/tests/docs cho khớp.

## Các bước (theo thứ tự, mỗi bước build + test xanh trước khi sang bước sau)

### Bước 1 — Dọn dependency & scaffold
- Đổi `go.mod`: bỏ `bbolt`, thêm `modernc.org/sqlite` (pure-Go, không CGO).
- `.gitignore`: `shortlink`, `*.db`, `data/`.
- **Verify**: `go mod tidy` sạch.

### Bước 2 — `internal/shortener` (counter + Feistel → base62 cố định 6)
- Không gian: `N = 62⁶`. Mọi ID hợp lệ ∈ `[0, N)`.
- `obfuscate(id) → x`: Feistel network (vài vòng, key cố định) hoán vị song ánh trên `[0, N)`. `deobfuscate(x) → id` là nghịch đảo (Feistel chạy ngược).
- `Encode(id uint64) (string, error)`: `obfuscate(id)` → base62 pad **đúng 6 ký tự**. Lỗi nếu `id ≥ N`.
- `Decode(code string) (uint64, error)`: base62 → x → `deobfuscate(x)` → id. Reject độ dài ≠ 6 hoặc ký tự ngoài alphabet.
- **Test**: round-trip `Decode(Encode(id)) == id` cho nhiều id (0, 1, biên N-1); id liền nhau (0,1,2) → code KHÁC hẳn nhau (chứng minh không enumerable); luôn ra 6 ký tự; reject code sai định dạng; `id ≥ N` báo lỗi.

### Bước 3 — `internal/store` (SQLite)
- Schema: `CREATE TABLE IF NOT EXISTS links (id INTEGER PRIMARY KEY AUTOINCREMENT, url TEXT NOT NULL UNIQUE, created_at ...)`.
- `Save(url) (id, error)`: insert; nếu URL đã tồn tại (unique violation) → trả id cũ (idempotent). Dùng `INSERT ... ON CONFLICT(url) DO UPDATE ... RETURNING id` hoặc lookup-then-insert.
- `GetURL(id) (url, bool)`: tra theo id.
- Parameterized query (chống SQLi).
- **Test**: DB tạm (`t.TempDir()`); save→get; save cùng URL 2 lần → cùng id; **đóng rồi mở lại DB, get vẫn ra** (R6 — persist sau restart).

### Bước 4 — `internal/handler`
- Interface `Store` (consumer-side) để mock.
- `POST /encode`: parse JSON → validate URL (non-empty, ≤2048, http/https, có host, **không chứa ký tự control/CRLF**) → `store.Save` → `shortener.Encode(id)` → trả `{"short_url": baseURL + "/" + code}` status 201.
- `POST /decode`: parse JSON → tách code từ short_url → **validate code length=6 + charset base62 trước khi deobfuscate** → `shortener.Decode` → `store.GetURL(id)` → trả `{"original_url": ...}` 200, hoặc 404.
- `GET /{code}` (bonus): redirect 302.
- `GET /health`.
- **Security hardening**:
  - `http.MaxBytesReader` giới hạn body (chống DoS).
  - Header `X-Content-Type-Options: nosniff` trên mọi response.
  - Không log full URL ở mức info (tránh rò rỉ qua log).
- Helper `writeJSON` / `writeError` (lỗi 5xx không lộ chi tiết nội bộ).
- **Test**: httptest + mock store. Happy path encode/decode; idempotent; lỗi (body rỗng, JSON sai, scheme sai, URL quá dài, URL có ký tự control, decode code sai định dạng, decode code không tồn tại); content-type JSON; round-trip encode→decode.

### Bước 5 — `cmd/server/main.go`
- Đọc env (`PORT`, `DB_PATH`, `BASE_URL`, `FEISTEL_KEY`), default hợp lý.
- Mở store, wiring handler, `http.Server` có timeouts, graceful shutdown (SIGINT/SIGTERM).
- **Verify**: `go build ./...` xanh; chạy thật + curl round-trip.

### Bước 6 — Tài liệu (VIẾT LẠI — bản hiện tại đang stale: nói BoltDB/hash)
- `README.md`: overview, API, **Security (attack vectors — bảng đầy đủ ở audit)**, **Scalability**. Trình bày collision = *loại bằng song ánh Feistel*, KHÔNG còn nói hash collision; scale thật = sinh ID phân tán + hết không gian 62⁶ → tăng độ dài.
- `RUNNING.md`: local, Docker, test, curl. Cập nhật cho SQLite (bỏ mọi nhắc tới BoltDB).
- `Dockerfile` + `Makefile`: SQLite pure-Go → vẫn `CGO_ENABLED=0` build tĩnh OK.
- `.gitignore`: binary, `*.db`, `data/`.

### Bước 7 — Verify tổng & Git
- `go vet ./...`, `gofmt`, `go test -race ./...` toàn xanh.
- Chạy server, test round-trip qua curl, **restart và decode lại** để chứng minh R6.
- `git init`, commit theo từng nhóm, push GitHub.

### Bước 8 — Deploy (sau, theo D-g)
- Chọn free server, deploy Docker, persist volume cho file SQLite.
- Lấy URL live demo.

### Bước 9 — Email nộp bài (D7)
- Soạn nội dung trả lời email assignment, kèm: URL GitHub repo, URL live demo, link README/RUNNING, tóm tắt thiết kế (SQLite + Feistel, security, scalability).
- (Tôi soạn nội dung; bạn review trước khi gửi.)

## Chiến lược Test (Unit + E2E + Coverage)

### Tầng 1 — Unit test (mock/isolated, nhanh)
| Package | Phủ gì |
|---|---|
| `internal/shortener` | Feistel round-trip `Decode(Encode(id))==id` (id=0,1,N-1, ngẫu nhiên); id liền nhau → code khác hẳn (không enumerable); luôn 6 ký tự; reject code sai format; `id≥N` lỗi |
| `internal/store` | Save→Get; idempotent (cùng URL→cùng id); not-found; **persist sau khi Close + mở lại DB** (R6); parameterized query không vỡ với URL có dấu nháy `'` (chống SQLi) |
| `internal/handler` | Mock store qua interface. Happy path encode/decode; idempotent; mọi error case (body rỗng, JSON sai, scheme sai, URL quá dài, URL có ký tự control, code sai format, code không tồn tại); content-type JSON; header nosniff |

### Tầng 2 — E2E / Integration test (server thật + SQLite thật)
- File `internal/e2e` hoặc `cmd/server` test: dựng `httptest.Server` với handler thật + store SQLite trên `t.TempDir()`.
- Kịch bản full flow:
  1. `POST /encode` URL thật → nhận short_url 6 ký tự.
  2. `POST /decode` short_url đó → đúng URL gốc (round-trip qua HTTP thật).
  3. `GET /{code}` → 302 + `Location` = URL gốc.
  4. Encode lại cùng URL → cùng short_url (idempotent qua HTTP).
  5. **Đóng store, mở lại trên cùng file DB, decode lại → vẫn đúng** (R6 end-to-end).
  6. Error path qua HTTP: decode code lạ → 404 JSON.

### Tầng 3 — Coverage
- Mục tiêu: **≥ 85%** trên 3 package `internal/*` (logic chính). `cmd/server/main.go` (wiring/os.Exit) khó phủ → không tính cứng.
- Lệnh: `go test -race -coverprofile=cover.out ./... && go tool cover -func=cover.out`.
- Thêm Makefile target `cover` in tổng % và `cover-html` mở report.
- **Verify ở Bước 7**: in coverage thực tế, nếu package internal < 85% thì bổ sung case.

## Định nghĩa "Done"
- [ ] `/encode` + `/decode` trả JSON, round-trip đúng.
- [ ] Persist sau restart (có test unit + e2e chứng minh).
- [ ] Unit test 3 package + **E2E test** server thật, `go test -race ./...` xanh.
- [ ] Coverage internal/* **≥ 85%** (in bằng `make cover`).
- [ ] README có Security + Scalability/collision (khớp thiết kế Feistel).
- [ ] RUNNING.md hướng dẫn chạy.
- [ ] Push GitHub + deploy demo.
- [ ] Soạn email nộp bài (D7).
