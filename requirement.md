# Requirements — ShortLink

> Bản nắm yêu cầu từ đề bài assignment. Đây là "source of truth" để mọi quyết định kỹ thuật bám theo.

## 1. Mục tiêu

Xây dựng một dịch vụ rút gọn URL (URL shortening service).
- Input: một URL dài, ví dụ `https://codesubmit.io/library/react`
- Output: một URL ngắn, ví dụ `http://your.domain/GeAi9K`
- Và ngược lại: từ URL ngắn lấy lại được URL gốc.

## 2. Yêu cầu bắt buộc (MUST)

| # | Yêu cầu | Nguồn (đề bài) |
|---|---------|----------------|
| R1 | Ngôn ngữ: **Golang**. Framework tùy chọn (hoặc không dùng). | "Language: Golang" |
| R2 | Endpoint `/encode`: encode URL gốc → URL ngắn. | "Two endpoints are required" |
| R3 | Endpoint `/decode`: decode URL ngắn → URL gốc. | "Two endpoints are required" |
| R4 | Cả hai endpoint trả về **JSON**. | "Both endpoints should return JSON format" |
| R5 | Encode rồi Decode phải khôi phục đúng URL gốc (round-trip). | "a URL can be encoded ... and ... decoded back" |
| R6 | **Persistence**: decode được các URL đã encode kể cả **sau khi restart** ứng dụng. | "able to decode previously encoded URLs after a restart" |
| R7 | File markdown riêng hướng dẫn cách chạy. | "instructions on how to run ... in a separate markdown file" |
| R8 | Có **test** cho cả hai endpoint (và test khác nếu cần). | "Provide tests for both endpoints" |
| R9 | Tài liệu **attack vectors** trong README. | "think through potential attack vectors ... document them in the README" |
| R10 | Tài liệu hướng tiếp cận **scale + collision problem** trong README. | "how your implementation may scale up ... collision problem ... document" |

## 3. Yêu cầu không bắt buộc / ngoài phạm vi (OUT OF SCOPE)

- KHÔNG cần build một dịch vụ thực sự scale được. ("You do not need to build a scalable service")
- Chỉ cần *tài liệu hóa* hướng scale, không cần triển khai phân tán thật.

## 4. Deliverables (sản phẩm nộp)

| # | Hạng mục |
|---|----------|
| D1 | Source code Go, tổ chức sạch sẽ, ở trạng thái "demo-ready". |
| D2 | Push lên GitHub (public repo OK). |
| D3 | Deploy demo trên một free server. |
| D4 | File markdown hướng dẫn chạy (RUNNING.md). |
| D5 | README có: API, attack vectors, scalability/collision. |
| D6 | Test chạy được (`go test ./...`). |
| D7 | Trả lời email kèm: URL git repo, URL live demo, tài liệu liên quan. |

## 5. Tiêu chí đánh giá (Evaluation Criteria — bám sát khi review)

1. **Golang best practices** — code idiomatic, layout chuẩn.
2. **API** — có `/encode` và `/decode`.
3. **Completeness** — đủ feature, test chạy hết.
4. **Correctness** — hành xử hợp lý, có suy nghĩ.
5. **Maintainability** — code sạch, dễ bảo trì.
6. **Security** — nhận diện + mitigate/document được vấn đề.
7. **Scalability** — nhận diện vấn đề scale + cách xử lý.

## 6. Câu hỏi mở / quyết định cần chốt (DECISIONS)

Những điểm đề bài không quy định, cần tự quyết và ghi lý do:

- **D-a. Storage gì?** → đề chỉ yêu cầu persist sau restart. Lựa chọn: embedded KV (BoltDB) vs SQLite vs file JSON vs external (Postgres/Redis). Tiêu chí: đơn giản, không cần service ngoài, persist được, demo-ready.
- **D-b. Sinh short code thế nào?** → hash-based (deterministic) vs counter base62 vs random. Liên quan trực tiếp tới "collision problem" cần document.
- **D-c. HTTP method cho /encode, /decode?** → POST với JSON body (RESTful, an toàn cho URL có ký tự đặc biệt).
- **D-d. Độ dài short code?** → cân bằng giữa ngắn gọn và không gian mã (collision).
- **D-e. Có cần GET /{code} redirect không?** → đề không yêu cầu, nhưng là hành vi tự nhiên của URL shortener. Cân nhắc thêm như bonus.
- **D-f. Deploy ở đâu?** → free tier: Render / Fly.io / Railway. Cần Dockerfile.

## 7. Quyết định đã chốt (LOCKED)

| Mã | Quyết định | Lý do |
|----|-----------|-------|
| D-a | **Storage = SQLite** (pure-Go driver `modernc.org/sqlite`, không cần CGO). | Persist sau restart; query SQL rõ ràng; dễ thêm unique index để dedup; single-file, demo-ready. |
| D-b | **Short code = Counter + obfuscation song ánh (Feistel) → base62 cố định 6 ký tự**. ID tự tăng → hoán vị song ánh trên `[0, 62⁶)` → base62 pad 6 ký tự. | Không collision (song ánh), không enumerable (Feistel trộn ID), khớp ví dụ đề `GeAi9K`, decode bằng phép nghịch đảo (không cần bảng reverse). Đây là best — ăn cả 3 tiêu chí Correctness/Security/Scalability. |
| D-c | **Idempotency = UNIQUE index trên `url`** + lookup trước insert. | Cùng URL encode nhiều lần trả cùng short code, không tạo rác. |
| D-d | **HTTP = POST + JSON body** cho `/encode`, `/decode`. | An toàn cho URL có ký tự đặc biệt; RESTful. |
| D-e | **Short code length = 6 ký tự cố định** (pad base62). Không gian = 62⁶ ≈ 56.8 tỷ. | Khớp ví dụ đề; đủ lớn cho demo và xa hơn. |
| D-f | **GET /{code} redirect**: thêm như bonus (302). | Hành vi tự nhiên của URL shortener; không bắt buộc nhưng tăng điểm correctness. |
| D-g | **Deploy**: quyết định sau khi local chạy ổn. | Ưu tiên code + repo trước. |

> Lưu ý: phần Scalability/README sẽ trình bày: collision được **loại bỏ bằng song ánh** (không phải vá hash); vấn đề scale thật là **sinh ID duy nhất phân tán** (range allocation / Snowflake / Redis INCR) và việc hết không gian 62⁶ → tăng độ dài code.
