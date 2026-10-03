# Learning Log

Daily notes while building raftkv. Plan: `../Distributed_KV_Store_Project_Plan.pdf`.

## Day 1 — 2026-10-03

**Done**
- Ran `go mod init github.com/edmund1472/raftkv` and created `go.mod`
- Hello world in `cmd/raftkv/main.go`, ran with `go run ./cmd/raftkv`
- Tried `go doc fmt.Println`

**Learned**
- `go.mod` marks the root of one Go project (module): its name and dependencies. It goes at the repo root so `cmd/` and `internal/` are both inside it.
- One folder = one package. Runnable programs are `package main` with `func main()`.
- `cmd/<name>/` holds each runnable program; `internal/` holds shared code that only this project can import.
- Reading signatures: `...any` = variadic, any type; Go functions can return multiple values, usually `(result, error)`.
- Capitalized names are exported (public). Errors are returned and checked with `if err != nil`.

**Next (Day 2)**
- Finish Tour of Go: Basics, then Methods and interfaces
- Map exercise: PUT / GET / DELETE with a `map[string]string`
- Create `internal/store` with a `Store` interface and an in-memory implementation
