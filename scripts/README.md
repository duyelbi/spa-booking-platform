# Scripts — Postgres migrate & backup (Docker)

## Makefile (repo root)

Không cần gọi `./scripts/...` trực tiếp nếu dùng Make:

```bash
make help          # danh sách target
make up            # docker compose up -d --build
make down          # docker compose down
make logs          # logs -f (một service: make logs s=api)
make migrate-up
make db-shell      # psql (dùng scripts/db-shell.sh)
make backup
make api-rebuild   # chỉ rebuild service api (gọi scripts/docker-rebuild-api.sh)
make restore FILE=backups/postgres_....sql.gz
```

## Docker: rebuild API only

Sau khi sửa code Go/Dockerfile của service `api`:

```bash
make api-rebuild
# hoặc: ./scripts/docker-rebuild-api.sh
```

Chạy `docker compose build api` rồi `up -d api`. Postgres/Redis không rebuild; volume DB giữ nguyên.

## Schema migrations (SQL)

- **Tự động:** Khi container `api` khởi động, server gọi `db.Migrate()` (file SQL trong `services/api/internal/db/migrations/`).
- **Chỉ chạy migrate (không bật API):**

  ```bash
  docker compose --profile tools run --rm migrate
  ```

  Cần Postgres healthy (`docker compose up -d postgres`). Dùng cùng `DATABASE_URL` nội bộ như service `api`.

- **Trên máy (không Docker API):**

  ```bash
  cd services/api && go run ./cmd/migrate
  ```

  `.env` ở root repo phải có `DATABASE_URL` hoặc đủ biến `POSTGRES_*` (xem `internal/config`).

## Backup

```bash
chmod +x scripts/db-backup.sh   # một lần
./scripts/db-backup.sh
```

Tạo file nén `backups/postgres_<db>_<timestamp>.sql.gz` (thư mục `backups/` đã được `.gitignore`).

Yêu cầu: `docker compose up -d postgres` và file `.env` có `POSTGRES_USER`, `POSTGRES_DB`, `POSTGRES_PASSWORD`.

## Restore

```bash
chmod +x scripts/db-restore.sh
./scripts/db-restore.sh backups/postgres_spa_booking_YYYYMMDD_HHMMSS.sql.gz
```

Dump từ `db-backup.sh` dùng `--clean --if-exists`: restore có thể **xóa và tạo lại** object trong DB — chỉ chạy khi chắc chắn.

## Redis (tùy chọn)

Dữ liệu Redis nằm trong volume `redisdata`. Backup nhanh khi stack đang chạy:

```bash
docker compose exec redis redis-cli SAVE
docker run --rm -v spa-booking-platform_redisdata:/data -v "$(pwd)/backups:/out" alpine \
  cp /data/dump.rdb "/out/redis_$(date +%Y%m%d_%H%M%S).rdb"
```

Tên volume có thể khác; kiểm tra: `docker volume ls | grep redis`.
