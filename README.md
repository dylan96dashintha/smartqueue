# SmartQueue

**SmartQueue** is a high-performance, tenant-aware in-memory data structure written in Go.  
It efficiently handles time-to-live (TTL)-based data management with callback triggers,  
making it ideal for scenarios like temporary caching, rate-limiting, and event-based state handling in multi-tenant environments.

---

## Features

- **Multi-Tenant Aware** — Each tenant’s data and lifecycle are fully isolated.  
- **Predictable Performance** — Independent cleanup and heap management for each tenant avoid shared bottlenecks.  
- **Automatic Expiration** — Items are efficiently removed upon TTL expiry, freeing memory without manual cleanup.  
- **Callback-Driven Events** — Execute domain logic (like logging or notifications) on data expiry.  
- **Memory-Constrained Design** — Maintains a bounded queue capacity per tenant.  
- **Concurrent Safe** — Designed with fine-grained locking to handle high-concurrency workloads gracefully.  
- **Simple Integration** — Works as a plug-and-play in-memory data layer for any Go application.

---

## Performance Notes

SmartQueue is optimized for high concurrency and minimal CPU overhead:

- Uses **O(log n)** heap operations for efficient expiry scheduling.  
- Maintains **per-tenant goroutines** for expiry management, ensuring even workload distribution.  
- Avoids **expensive global locks** through isolated tenant stores.  
- Produces **minimal GC pressure** via structured object reuse and heap pruning.  
- Performs predictably under high load with concurrent enqueue/dequeue operations.

---

## Benchmark Results

All benchmarks were executed on a **Linux environment** using **Go 1.23**,  
on an **Intel(R) Core(TM) Ultra 7 155H** processor.

| Operation Type                | Iterations  | ns/op   | B/op | allocs/op |
|-------------------------------|-------------|---------|------|------------|
| Enqueue (TTL store)           | 1,067,413     | 1109    | 532  | 7          |
| Enqueue (Concurrent)          | 1,000,000   | 1129    | 515  | 7          |
| Pop                           | 20,353,551  | 55.96   | 0    | 0          |
| Enqueue + Dequeue             | 68,778,433  | 17.64   | 0    | 0          |
| Remove                        | 32,553,478  | 34.29   | 0    | 0          |

>  **PASS** — All benchmarks completed successfully in **8.699s**.

---

## Profiling Summary (via `pprof`)

- `Enqueue()` accounted for ~70% of total CPU time, primarily due to heap ordering and TTL scheduling.  
- Per-tenant locking **minimized contention** even under concurrent access.  
- **Memory footprint remained stable** under sustained high-frequency writes.  
- No **global blocking** observed in concurrent mixed workloads.

---

## Interpretation

SmartQueue maintains **high throughput** with **predictable latency**,  
even under multi-tenant, concurrent workloads.

- Heap-based expiry tracking introduces minimal overhead.  
- Per-tenant concurrency design scales **linearly** with the number of active tenants.  
- The design is suitable for **real-time, high-throughput systems** like driver caches, rate limiters, and transient session stores.

---

## Future Enhancements

- Built-in **metrics and observability** (Prometheus / OpenTelemetry).  
- Support for alternative **eviction policies** (LRU, LFU).  
- **Distributed SmartQueue** for multi-instance or cluster-level scaling.  
- **Priority-based queuing** and dynamic TTL adjustments.  

---

## License

**MIT License**  
You are free to use, modify, and distribute **SmartQueue** in both open-source and commercial projects.

---

### Example Use Case (coming soon...)

---

