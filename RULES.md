# Project Rules — ShortLink

> Quy ước kỹ thuật cho project. Mọi code/PR phải tuân thủ. Mục tiêu: code idiomatic, sạch, demo-ready, đúng tiêu chí đánh giá của assignment.

## 1. Layout & Cấu trúc

- Tuân theo [Standard Go Project Layout](https://github.com/golang-standards/project-layout) ở mức vừa đủ:
  - `cmd/server/` — entrypoint `main.go`, chỉ wiring + cấu hình.
  - `internal/` — toàn bộ logic, không export ra ngoài module.
    - `internal/shortener/` — thuật toán sinh short code (base62, encode ID).
    - `internal/store/` — tầng persistence (SQLite), thuần data access.
    - `internal/handler/` — HTTP handlers, request/response JSON, validation đầu vào.
- Mỗi package có một trách nhiệm rõ ràng (single responsibility). Không để handler chứa SQL, không để store biết về HTTP.

## 2. Dependency & Coupling

- Handler phụ thuộc store qua **interface** định nghĩa tại nơi sử dụng (consumer-side interface), không phụ thuộc struct cụ thể → dễ mock khi test.
- Không import vòng. `store` và `shortener` không được import `handler`.
- Giữ số lượng third-party dependency tối thiểu. Hiện tại: chỉ SQLite driver **`modernc.org/sqlite`** (pure-Go, CGO-free — xác minh: production-proven 2+ năm, cho phép `CGO_ENABLED=0` build tĩnh). Routing dùng `net/http` stdlib (`http.ServeMux` với pattern method, Go 1.22+). Test dùng `httptest` stdlib.

## 3. Error Handling

- Wrap lỗi với context: `fmt.Errorf("open db: %w", err)`. Không nuốt lỗi (`_ =`) trừ khi có lý do rõ ràng (ví dụ ghi JSON response đã set status).
- Handler không bao giờ lộ lỗi nội bộ ra client. Lỗi 5xx trả message chung chung; log chi tiết ở server.
- Lỗi validation đầu vào → 400 kèm message rõ ràng cho client.

## 4. HTTP / API

- Request & response **luôn JSON**, `Content-Type: application/json`.
- Status codes đúng ngữ nghĩa: `201` khi tạo mới, `200` khi đọc/decode, `400` input sai, `404` không tìm thấy, `500` lỗi server.
- Validate đầu vào trước khi chạm DB: URL non-empty, ≤ 2048 ký tự, scheme `http`/`https`, có host.
- Giới hạn kích thước body (`http.MaxBytesReader`) để chống DoS.
- Server có `ReadTimeout` / `WriteTimeout` / `IdleTimeout` và **graceful shutdown** (SIGINT/SIGTERM).

## 5. Persistence

- SQLite, schema khởi tạo idempotent (`CREATE TABLE IF NOT EXISTS`).
- Dùng prepared statement / parameterized query — **không** string-concat SQL (chống SQL injection).
- Mọi mapping phải khôi phục được sau restart (yêu cầu R6). Test phải chứng minh điều này.

## 6. Naming & Style

- `gofmt` / `goimports` sạch (CI/Makefile có thể kiểm). `go vet` không cảnh báo.
- Tên export có doc comment bắt đầu bằng tên định danh (Go convention).
- Không viết tắt khó hiểu. Hằng số (alphabet base62, độ dài, giới hạn) đặt tên rõ, không magic number rải rác.

## 7. Testing

- Mỗi package có test riêng, **table-driven** khi có nhiều case.
- `internal/shortener`: test thuật toán base62 (round-trip encode/decode ID, tính duy nhất).
- `internal/store`: test với DB tạm (`t.TempDir()`), test cả **persist sau khi đóng/mở lại** DB (R6).
- `internal/handler`: dùng `httptest`, mock store qua interface. Phủ happy-path + error cases (body rỗng, JSON sai, URL sai scheme, decode code không tồn tại).
- `go test -race ./...` phải xanh.

## 8. Config

- Cấu hình qua biến môi trường, có default hợp lý: `PORT`, `DB_PATH`, `BASE_URL`.
- Không hardcode secret/đường dẫn tuyệt đối.

## 9. Tài liệu

- `README.md`: overview, API, **Security (attack vectors)**, **Scalability (collision problem)**.
- `RUNNING.md`: hướng dẫn chạy local, Docker, test, ví dụ curl.
- `requirement.md`: bản nắm yêu cầu (không sửa trừ khi yêu cầu đổi).
- Mỗi quyết định kiến trúc quan trọng phải có lý do ghi lại (trong requirement.md mục 7 hoặc README).

## 10. Commit / Git

- Commit nhỏ, message rõ theo thể: `feat:`, `fix:`, `test:`, `docs:`, `chore:`.
- Không commit file build (`shortlink` binary), file DB (`*.db`), thư mục `data/`. → `.gitignore`.
