# brothers-app

**brothers-app** is a backend application in Go built following Clean Architecture principles.

- **Fiber v2** as a fast and lightweight web framework for RESTful APIs
- **sqlc + pgx** for type-safe PostgreSQL access
- **JWT (JSON Web Tokens)** for secure stateless authentication
- **Swagger** for interactive REST API documentation
- **Docker Compose** for easy setup of development and test PostgreSQL databases

## Features

- Clear separation of concerns with Clean Architecture (`entities`, `usecase`, `repository`, `handler/rest`, `dto`)
- High-performance HTTP handling with Fiber v2
- Explicit, type-safe database integration using sqlc and pgx
- JWT-based authentication and protected endpoints
- Data Transfer Objects (DTO) to manage data structure transformations between layers
- Automatic Swagger API documentation at `/api/v1/docs`
- Ready-to-use Docker Compose setup for dev and test databases

## Getting Started

Follow the steps below to set up and run the project:

### 1. Install Go module dependencies

```bash
go mod tidy
```

### 2. Configure environment variables

```bash
cp .env.example .env.dev
```

Configure your `.env.dev` file with your database and application settings:
- Development database: `DB_NAME`, `DB_USER`, `DB_PASSWORD`, `DB_PORT`
- Test database: `DB_TEST_NAME`, `DB_TEST_USER`, `DB_TEST_PASSWORD`, `DB_TEST_PORT`
- Application settings: `APP_PORT`, `JWT_SECRET`, `JWT_EXPIRATION`

### 3. Start PostgreSQL with Docker Compose

```bash
# Start both development and test databases
docker-compose --env-file .env.dev up -d

# Or start only the development database
docker-compose --env-file .env.dev up -d postgres

# Or start only the test database
docker-compose --env-file .env.dev up -d postgres-test
```

### 4. Run the application

```bash
go run ./cmd/api
```

The application will start on port `8000` by default (configurable via `APP_PORT` in `.env.dev`).

### 5. API Documentation

Interactive Swagger UI is accessible at:

```
http://localhost:8000/api/v1/docs
```

To regenerate Swagger documentation after modifying route annotations:

```bash
swag init -g cmd/api/main.go -o docs/v1
```

### 6. Run tests

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests with coverage
go test -v -coverprofile=coverage.out ./...
```

See [docs/TESTING.md](docs/TESTING.md) for details on the test suite and database isolation.

## Deploy to Vercel

The Go entrypoint is `cmd/api/main.go`, which Vercel detects automatically. No
custom build command or `vercel.json` is required.

Set these environment variables in the Vercel project before deploying:

- `APP_ENV=production`
- `DATABASE_URL`: hosted PostgreSQL connection URL, including the provider's TLS settings
- `REDIS_URL`: hosted Redis connection URL used for OTP records
- `JWT_SECRET`: a strong random signing secret
- `OTP_PEPPER`: a separate strong random secret for hashing OTP values
- `CORS_ALLOW_ORIGINS`: comma-separated frontend origins allowed to call the API
- `CORS_ALLOW_CREDENTIALS`: set to `true` only with explicit origins, never with `*`
- `TELEGRAM_BOT_TOKEN` and `TELEGRAM_BOT_USERNAME` when Telegram auth is enabled

Do not set `PORT` in Vercel; the platform injects it automatically.

### Run production database migrations

Production migrations run separately from the Vercel application process.
Configure them once in GitHub:

1. Open `Settings` -> `Environments` and create the `production` environment.
2. Add `DATABASE_URL` as an environment secret. Use the hosted PostgreSQL URL
   with its required TLS parameters.
3. Optionally configure required reviewers for the `production` environment.
4. Open `Actions` -> `Database Migration` -> `Run workflow` and select the
   `main` branch.
5. Redeploy Vercel only after the migration workflow succeeds.

The workflow applies only pending `*.up.sql` files and serializes production
migration runs. It never runs a down migration. Do not run the current first
down migration against production because it drops the application tables.

For each subsequent schema change, add a new numbered pair such as
`000002_feature.up.sql` and `000002_feature.down.sql`. Never edit a migration
that has already been applied to production.

## Environment Variables

Key environment variables in `.env.dev`:

### Application Settings
- `PORT`: HTTP server port assigned automatically by Vercel; it takes precedence over `APP_PORT`
- `APP_PORT`: Local HTTP server port (default: `8000`)
- `APP_ENV`: Application environment (e.g. `development`, `test`)
- `JWT_SECRET`: Secret key for JWT token signing
- `JWT_EXPIRATION`: JWT token expiration in seconds (default: `3600`)
- `CORS_ALLOW_ORIGINS`: comma-separated allowed browser origins (default: `*`)
- `CORS_ALLOW_CREDENTIALS`: whether credentialed browser requests are allowed (default: `false`)

### Development Database
- `DATABASE_URL`: Hosted PostgreSQL connection URL; it takes precedence over the individual `DB_*` values
- `DB_HOST`: Database host (default: `localhost`)
- `DB_PORT`: Database port (default: `5432`)
- `DB_USER`: Database user (default: `postgres`)
- `DB_PASSWORD`: Database password
- `DB_NAME`: Database name

### Redis
- `REDIS_URL`: Redis connection URL used for OTP storage; use a hosted Redis instance in production

### Test Database
- `DB_TEST_HOST`: Test database host (default: `localhost`)
- `DB_TEST_PORT`: Test database port (default: `5433`)
- `DB_TEST_USER`: Test database user
- `DB_TEST_PASSWORD`: Test database password
- `DB_TEST_NAME`: Test database name

## Project Structure

```bash
/brothers-app
├── cmd/
│   └── api/
│       └── main.go                 # Entrypoint
├── docs/
│   ├── TESTING.md
│   └── v1/                         # Swagger generated docs
├── internal/
│   ├── app/
│   │   ├── app.go                  # Fiber app & DB dependency setup
│   │   └── server.go               # Server lifecycle & graceful shutdown
│   ├── entities/                   # Domain entities (User, Order)
│   ├── order/                      # Order feature module
│   │   ├── dto/                    # DTOs & mappers
│   │   ├── handler/
│   │   │   └── rest/               # Fiber REST controller
│   │   ├── repository/             # sqlc repository implementation
│   │   └── usecase/                # Business logic & tests
│   └── user/                       # User & Auth feature module
│       ├── dto/
│       ├── handler/
│       │   └── rest/
│       ├── repository/
│       └── usecase/
├── pkg/
│   ├── apperror/                   # Standard error handling
│   ├── config/                     # Environment configuration loader
│   ├── database/                   # DB connection & test helpers
│   ├── middleware/                 # Fiber & JWT middlewares
│   ├── responses/                  # Response formatters
│   └── routes/                     # Route definitions (public, private, docs)
├── utils/                          # Server runners & shutdown listeners
├── .env.example                    # Sample environment variables
├── .gitignore
├── docker-compose.yaml             # PostgreSQL containers for dev/test
└── go.mod
```
