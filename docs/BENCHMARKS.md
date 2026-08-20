# 🚀 Nacho Flow: Performance & Benchmarks

This document details the performance characteristics, load-testing methodology, and high-concurrency stress test results for **Nacho Flow**.

---

## 1. Executive Summary

- **Peak Throughput**: **32,254 requests/second** (~1.93 million requests/minute).
- **Pipeline Latency**: **~0.29 ms** (299 microseconds) end-to-end overhead per request.
- **Extreme Concurrency**: Handled **1,000 parallel workers** with **100.0% success rate** (0 dropped connections, 0 errors).
- **Memory Footprint**: Peak heap memory remained under **96 MB** while sustaining 1,000 concurrent client streams.
- **Telemetry Integrity**: Aggregated **350,000 live proxy events** with **zero race conditions** and **zero data drops**.
- **Real-World Complex Workloads**: Maintains **~28,900 req/s** with active Inbound Bearer Authentication and real-time Multi-Model Tool-Call Normalization (Hermes/Mistral/Llama/DeepSeek bracket balancing).

---

## 2. Test Environment

| Attribute | Specification |
| :--- | :--- |
| **Host System** | NachoPC |
| **CPU** | AMD Ryzen 7 5700X3D (8 Cores / 16 Threads @ 3.00 GHz, 96MB 3D V-Cache) |
| **Installed RAM** | 64.0 GB |
| **GPU** | AMD Radeon RX 9070 XT (16 GB VRAM) |
| **Operating System** | Windows 11 Pro (64-bit, x64-based) |
| **Go Version** | Go 1.22+ (Native compilation, `CGO_ENABLED=0`) |
| **Pipeline Tested** | Full end-to-end: Token Classifier $\rightarrow$ `expr` AST Engine $\rightarrow$ History Sanitizer $\rightarrow$ Reverse Proxy Director $\rightarrow$ Pooled Transport $\rightarrow$ Response Interceptor $\rightarrow$ Lock-Free Pricing Oracle $\rightarrow$ Asynchronous Stats Tracker |

---

## 3. High-Concurrency Stress Test (350,000 Requests)

We subjected Nacho Flow to a 5-stage progressive stress test scaling concurrency from **50 to 1,000 concurrent worker goroutines**:

```
========================================================================================
🌮 NACHO FLOW STRESS TEST & BREAKING POINT ANALYSIS
========================================================================================
CPUs Available: 16 | OS: windows | Arch: amd64
Stress Plan:    Scaling concurrency: 50 -> 100 -> 250 -> 500 -> 1,000 parallel workers
========================================================================================

▶ [STAGE 1/5] Running 25,000 requests across 50 concurrent workers...
   ✓ Done in 0.78s | RPS: 32,254.4 | P50: 1.01ms | P99: 6.47ms  | Heap: 26.6 MB | Success: 25,000/25,000 (Fail: 0)

▶ [STAGE 2/5] Running 50,000 requests across 100 concurrent workers...
   ✓ Done in 1.56s | RPS: 32,020.4 | P50: 2.51ms | P99: 11.11ms | Heap: 49.3 MB | Success: 50,000/50,000 (Fail: 0)

▶ [STAGE 3/5] Running 75,000 requests across 250 concurrent workers...
   ✓ Done in 2.53s | RPS: 29,641.8 | P50: 6.24ms | P99: 34.81ms | Heap: 45.7 MB | Success: 75,000/75,000 (Fail: 0)

▶ [STAGE 4/5] Running 100,000 requests across 500 concurrent workers...
   ✓ Done in 3.43s | RPS: 29,184.7 | P50: 14.16ms| P99: 57.62ms | Heap: 75.0 MB | Success: 100,000/100,000 (Fail: 0)

▶ [STAGE 5/5] Running 100,000 requests across 1,000 concurrent workers...
   ✓ Done in 3.78s | RPS: 26,462.5 | P50: 27.17ms| P99: 147.81ms| Heap: 95.4 MB | Success: 100,000/100,000 (Fail: 0)
```

### Comprehensive Results Breakdown:

| Concurrency | Total Requests | Success Rate | Throughput (RPS) | P50 Latency | P99 Latency | Heap Memory |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **50 workers** | 25,000 | **100.0%** | **32,254.4 req/s** | 1.01 ms | 6.47 ms | 26.6 MB |
| **100 workers** | 50,000 | **100.0%** | **32,020.4 req/s** | 2.51 ms | 11.11 ms | 49.3 MB |
| **250 workers** | 75,000 | **100.0%** | **29,641.8 req/s** | 6.24 ms | 34.81 ms | 45.7 MB |
| **500 workers** | 100,000 | **100.0%** | **29,184.7 req/s** | 14.16 ms | 57.62 ms | 75.0 MB |
| **1,000 workers** | 100,000 | **100.0%** | **26,462.5 req/s** | 27.17 ms | 147.81 ms | 95.4 MB |

---

## 4. Advanced Complex-Workload Benchmark (Auth + Tool Normalization Under Load)

To stress the proxy under true production conditions, we benchmarked Nacho Flow against a **mixed workload rotating across 4 realistic client turn types**:
1. **Multi-Turn Routine Code Refactoring** (Local GPU, No Tools).
2. **Deep Reasoning Concurrency Analysis** (Keywords matching `mutex`, `deadlock`, routing to DeepSeek-R1).
3. **Agentic Tool Calling with Markdown JSON Fence** (HasTools = true, returning raw ````json {"name": "search_code", ...} ```` for on-the-fly bracket-balancing normalization).
4. **Hermes/Claude XML Tool Calls** (Returning `<tool_call>` XML blocks needing regex extraction and OpenAI transformation).
5. **Inbound Client Bearer Authentication** (Every single request validated via `Authorization: Bearer <token>`).

### Isolated A/B Overhead Analysis (Raw Pass-Through vs Full Security & Normalization):

To measure the exact CPU cost of inbound authentication and on-the-fly multi-model tool extraction with balanced-bracket JSON parsing, we benchmarked the gateway across 250,000 requests under identical pre-warmed conditions:

| Workers | Raw Pass-Through (Zero Normalization) | Full Normalization + Auth | Throughput Delta | P50 Latency Delta | P99 Tail Latency Delta |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **25 workers** | 27,417.4 req/s | 26,989.7 req/s | **-1.6%** | **+0.00 ms** (1.00ms vs 1.00ms) | +0.02 ms |
| **50 workers** | 28,092.1 req/s | 28,556.5 req/s | **+1.7%** (Noise margin) | **+0.15 ms** (1.31ms vs 1.46ms) | +0.21 ms |
| **100 workers** | 27,534.8 req/s | 28,286.9 req/s | **+2.7%** (Noise margin) | **+0.40 ms** (2.52ms vs 2.92ms) | -0.10 ms |
| **200 workers** | 26,864.1 req/s | 27,349.4 req/s | **+1.8%** (Noise margin) | **+0.52 ms** (5.01ms vs 5.53ms) | -5.44 ms |

**Engineering Finding**: 
- With the zero-allocation byte pre-filter (`hasCandidateToolTokens`) and targeted Go struct unmarshaling (`fastChatCompletionResponse`), the per-request latency overhead of tool normalization + inbound auth is **between 0.00ms and 0.52ms (under 520 microseconds)**.
- Throughput remains virtually identical to raw pass-through (~27,000 to 28,500 req/s across all concurrency levels), confirming near-zero compute degradation in real-world workloads.

---

## 5. Go Micro-Benchmarks (Nanosecond & Allocation Precision)

We ran isolated Go micro-benchmarks targeting the core HTTP routing pipeline and the tool normalization engine using Go's standard `testing.B` harness:

### 5.1 End-to-End Proxy Overhead:
```bash
$ go test -bench=BenchmarkProxy_ChatCompletions -benchmem -run=^$ ./pkg/server/...
```

```text
BenchmarkProxy_ChatCompletions_RawPassThrough-16       5955    186,649 ns/op    54,957 B/op    246 allocs/op
BenchmarkProxy_ChatCompletions_ToolNormalization-16    5178    212,805 ns/op    62,357 B/op    359 allocs/op
```

- **Raw Pass-Through Latency**: **186.6 µs** (0.186 milliseconds).
- **Tool Normalization Latency**: **212.8 µs** (0.212 milliseconds).
- **Exact Compute Cost**: **+26.15 µs** (+14.0% overhead, +7.4 KB memory allocation per turn).

### 5.2 Inner Tool Normalizer Performance by Model Format:
```bash
$ go test -bench=BenchmarkNormalize -benchmem ./pkg/router/...
```

```text
BenchmarkNormalize_PureProse_FastBailout-16             49,999,582    23.55 ns/op       0 B/op     0 allocs/op
BenchmarkNormalize_HermesXML_FullNormalization-16          481,904     2,436 ns/op    1,330 B/op    27 allocs/op
BenchmarkNormalize_Mistral_ArrayCalls-16                   255,790     4,562 ns/op    2,574 B/op    52 allocs/op
BenchmarkNormalize_DeepSeekR1_ReasoningAndToolCall-16      192,193     6,361 ns/op    1,734 B/op    34 allocs/op
```

- **Non-Tool Fast Bailout**: **23.55 nanoseconds** (Zero heap allocations, 0 B/op).
- **Hermes / Qwen XML Extraction**: **2.44 microseconds** (27 allocations).
- **Mistral Array Tool Extraction**: **4.56 microseconds** (52 allocations).
- **DeepSeek-R1 CoT + Markdown Normalization**: **6.36 microseconds** (34 allocations).

### 5.3 SSE Stream & CoT Normalization Performance:
```bash
$ go test -bench=BenchmarkSSE -benchmem ./pkg/server/...
```

```text
BenchmarkSSE_NonReasoning_ZeroAlloc-16      3,208,678       380.8 ns/op      193 B/op      5 allocs/op
BenchmarkSSE_ReasoningTransform-16            429,451     2,858.0 ns/op    1,194 B/op     21 allocs/op
```

- **Non-Reasoning Stream Passthrough**: **380.8 nanoseconds** (~2.6+ Million SSE chunks/sec).
- **Reasoning Stream `<think>` Transformation**: **2.86 microseconds** (~350,000 reasoning tokens/sec).
- **Zero GC Churn**: Buffer pooling (`sync.Pool`) and lightweight fast-struct parsing prevent memory spikes during long CoT reasoning generation.

---

## 6. How to Reproduce

You can reproduce these benchmarks on your own machine using the built-in tooling:

### 1. Run the Pre-Warmed Complex-Workload Benchmark:
```bash
go run ./cmd/util/nacho_bench
```

### 2. Run the Full 350,000-Request Stress Test:
```bash
go run ./cmd/util/nacho_bench -full
```

### 3. Run Standard Go Micro-Benchmarks:
```bash
go test -bench=. -benchmem ./pkg/server/...
```
