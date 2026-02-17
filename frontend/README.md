# Notery Frontend

A production-quality Reddit-style frontend for the Notery note marketplace API, built with **Next.js 14**, **TypeScript**, **TailwindCSS**, and **shadcn/ui**.

## Features

- **Reddit-like feed** – Hot, New, and Top sorting with infinite scroll
- **Note marketplace** – Browse, purchase, and download PDF notes
- **Voting system** – Reddit-style upvote/downvote on notes and comments
- **Threaded comments** – Nested comment threads with collapse, reply, edit, delete
- **User profiles** – Public profiles, settings, email verification
- **Shopping cart** – Add notes, checkout, purchase history
- **Multi-type search** – Search notes, communities, users, and comments
- **Dark mode** – Full light/dark theme support
- **Responsive** – Mobile-first 3-column layout
- **Authentication** – JWT-based with automatic token refresh

## Tech Stack

| Layer | Technology |
|---|---|
| Framework | Next.js 14 (App Router) |
| Language | TypeScript |
| Styling | TailwindCSS |
| Components | shadcn/ui (21 components) |
| State | Zustand (auth + feed stores) |
| Server State | @tanstack/react-query |
| Icons | Lucide React |
| Date Formatting | date-fns |
| Theme | next-themes |
| Testing | Jest + Testing Library |

## Getting Started

### Prerequisites

- Node.js 18+
- The Notery Go API running on `http://localhost:8080` (see root README)

### Install & Run

```bash
cd frontend
npm install
npm run dev
```

Open [http://localhost:3000](http://localhost:3000).

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `NEXT_PUBLIC_API_URL` | `http://localhost:8080` | Base URL of the Notery API |

### Available Scripts

| Command | Description |
|---|---|
| `npm run dev` | Start development server (port 3000) |
| `npm run build` | Production build |
| `npm start` | Start production server |
| `npm run lint` | Run ESLint |
| `npm test` | Run all tests |
| `npm run test:watch` | Run tests in watch mode |
| `npm run test:coverage` | Run tests with coverage report |

## Architecture

```
src/
├── app/                    # Next.js App Router pages
│   ├── page.tsx            # Home (hot feed)
│   ├── hot/                # Hot feed alias
│   ├── new/                # New feed alias
│   ├── login/              # Login page
│   ├── signup/             # Signup page
│   ├── forgot-password/    # Password reset request
│   ├── submit/             # Create note form
│   ├── notes/[id]/         # Note detail + comments
│   ├── search/             # Multi-type search
│   ├── cart/               # Shopping cart + checkout
│   ├── purchases/          # Purchase history
│   ├── profile/            # Own profile settings
│   └── user/[id]/          # Public user profile
├── components/
│   ├── feed/               # Feed components (note-card, vote-buttons, sort-tabs)
│   ├── comments/           # Threaded comment system
│   ├── layout/             # Top nav, left sidebar, right sidebar
│   ├── ui/                 # shadcn/ui components
│   ├── providers.tsx       # App-level providers (Query, Theme, Auth)
│   └── theme-provider.tsx  # Dark mode wrapper
├── hooks/
│   └── use-toast.ts        # Toast notification hook
├── lib/
│   ├── api-client.ts       # HTTP client with auto token refresh
│   ├── config.ts           # Environment configuration
│   ├── format.ts           # Price, date, vote formatting
│   └── utils.ts            # Tailwind class merger
├── services/               # API service layer (one file per domain)
│   ├── auth.ts             # Login, signup, refresh, password reset
│   ├── notes.ts            # CRUD, voting, PDF upload
│   ├── comments.ts         # Threaded comments, voting
│   ├── profile.ts          # User profiles
│   ├── purchases.ts        # Cart, checkout, purchase history
│   ├── search.ts           # Multi-type search
│   └── subnoteries.ts      # Community operations
├── stores/                 # Zustand state management
│   ├── auth-store.ts       # User session state
│   └── feed-store.ts       # Feed preferences (sort, view mode)
└── types/
    └── api.ts              # TypeScript types mirroring Go API models
```

## Pages

| Route | Description | Auth |
|---|---|---|
| `/` | Home feed (hot) | Optional |
| `/hot` | Hot feed | Optional |
| `/new` | New/latest feed | Optional |
| `/notes/:id` | Note detail + comments | Optional |
| `/search?q=&type=` | Multi-type search | None |
| `/login` | Login | None |
| `/signup` | Signup | None |
| `/forgot-password` | Request password reset | None |
| `/submit` | Create a new note | Required |
| `/cart` | Shopping cart | Required |
| `/purchases` | Purchase history | Required |
| `/profile` | Own profile settings | Required |
| `/user/:id` | Public user profile | None |

## API Client

The API client (`src/lib/api-client.ts`) handles:

- **Bearer token injection** on all authenticated requests
- **Automatic 401 refresh** – on 401 response, refreshes the access token and retries
- **Singleton refresh promise** – prevents concurrent refresh attempts
- **Error normalization** – all API errors wrapped in `ApiRequestError` with status and body

## Testing

```bash
# Run all tests
npm test

# Watch mode
npm run test:watch

# With coverage
npm run test:coverage
```

**Test structure:**
- `src/lib/__tests__/` – API client, config, formatting utilities
- `src/stores/__tests__/` – Zustand store behavior
- `src/services/__tests__/` – Service layer API call verification

**54 tests** across 8 test suites covering:
- Token management (get, set, clear)
- Error handling (ApiRequestError)
- Formatting (prices, dates, votes, file sizes)
- Auth store (initialize, setUser, logout, clearAuth)
- Feed store (sort, time filter, view mode)
- All service functions (auth, notes, purchases)

## Color Scheme

Reddit-inspired orange theme with dark mode:

| Token | Light | Dark |
|---|---|---|
| Primary | `hsl(24, 100%, 50%)` | `hsl(24, 100%, 50%)` |
| Background | `hsl(0, 0%, 100%)` | `hsl(0, 0%, 7%)` |
| Card | `hsl(0, 0%, 100%)` | `hsl(0, 0%, 10%)` |
| Muted | `hsl(0, 0%, 96%)` | `hsl(0, 0%, 15%)` |

## License

See [LICENSE](../LICENSE) in root.
