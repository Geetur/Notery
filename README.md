# Notery

Notery is a marketplace for notes — a RESTful API built with Go that allows users to create, share, and manage notes within communities called "subnoteries."

<img width="1024" height="1024" alt="Scribbled _N_ logo in ink" src="https://github.com/user-attachments/assets/b7ec42b5-f272-4b4a-ac7d-d0b6e6a99394" />

## Features

- **User Authentication** — JWT-based signup and login
- **Note Management** — Create, view, approve, reject, and delete notes
- **Subnoteries** — Community-based note organization with admin controls
- **Shopping Cart** — Redis-backed cart system for purchasing notes
- **Full-Text Search** — Meilisearch integration for approved notes
- **Role-Based Access** — Global admins and subnotery-specific admins

## Tech Stack

| Component   | Technology                                                 |
| ----------- | ---------------------------------------------------------- |
| Language    | Go 1.25+                                                   |
| Framework   | [Gin](https://github.com/gin-gonic/gin)                    |
| Database    | PostgreSQL 16 (via GORM)                                   |
| Cache       | Redis 7                                                    |
| Search      | [Meilisearch](https://www.meilisearch.com/)                |
| Auth        | JWT (HS256)                                                |

## Project Structure

```
Notery/
├── cmd/
│   └── api/
│       └── main.go          # Application entrypoint
├── internal/
│   ├── database/
│   │   ├── database.go      # PostgreSQL initialization
│   │   ├── redis.go         # Redis initialization
│   │   └── meilisearch.go   # Meilisearch initialization
│   ├── handlers/
│   │   ├── auth.go          # Authentication handlers
│   │   ├── cart.go          # Shopping cart handlers
│   │   ├── note.go          # Note CRUD handlers
│   │   └── subnotery.go     # Subnotery management handlers
│   ├── middleware/
│   │   ├── auth.go          # JWT authentication middleware
│   │   └── admin.go         # Admin authorization middleware
│   └── models/
│       ├── note.go          # Note model
│       ├── subnotery.go     # Subnotery model
│       └── user.go          # User model
├── docker-compose.yml       # Local development services
├── go.mod
└── README.md
```

## Getting Started

### Prerequisites

- Go 1.25 or later
- Docker & Docker Compose (for local services)

### Environment Variables

Create a `.env` file in the project root:

```env
# PostgreSQL
DB_HOST=localhost
DB_PORT=5432
DB_USER=admin
DB_PASSWORD=yourpassword
DB_NAME=notery_db
DB_SSLMODE=disable
DB_TIMEZONE=UTC

# Redis
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=yourredispassword
REDIS_DB=0

# Meilisearch
MEILISEARCH_HOST=http://localhost:7700
MEILISEARCH_MASTER_KEY=yourmeilisearchkey
MEILISEARCH_INDEX=notes

# JWT
JWT_SECRET=your-super-secret-key
```

### Running Locally

1. **Start infrastructure services:**

   ```bash
   docker-compose up -d
   ```

2. **Run the API:**

   ```bash
   go run cmd/api/main.go
   ```

3. **Verify the server is running:**

   ```bash
   curl http://localhost:8080/health
   ```

## API Endpoints

### Public

| Method | Endpoint            | Description          |
| ------ | ------------------- | -------------------- |
| GET    | `/health`           | Health check         |
| POST   | `/api/v1/signup`    | Register a new user  |
| POST   | `/api/v1/login`     | Authenticate user    |

### Protected (Requires JWT)

| Method | Endpoint                  | Description                |
| ------ | ------------------------- | -------------------------- |
| GET    | `/api/v1/notes/:id`       | Get note by ID             |
| POST   | `/api/v1/notes`           | Create a new note          |
| GET    | `/api/v1/notes/approved`  | List all approved notes    |
| GET    | `/api/v1/cart`            | Get user's cart            |
| POST   | `/api/v1/cart`            | Add item to cart           |
| DELETE | `/api/v1/cart/:item_id`   | Remove item from cart      |

### Admin Only

| Method | Endpoint                      | Description          |
| ------ | ----------------------------- | -------------------- |
| GET    | `/api/v1/notes/pending`       | List pending notes   |
| PATCH  | `/api/v1/notes/:id/approve`   | Approve a note       |
| PATCH  | `/api/v1/notes/:id/reject`    | Reject a note        |
| DELETE | `/api/v1/notes/:id`           | Delete a note        |

## Testing

Run unit tests:

```bash
go test ./...
```

## License
Copyright (c) 2026 Jeter Pontes. All rights reserved.

This repository is proprietary. No permission is granted to use, copy, modify, or distribute this code without explicit written permission.


