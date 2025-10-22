# TritonTube Streaming Platform Architecture

This document captures the end-to-end design for TritonTube's upload, transcoding, and playback surfaces, including the Web API contract, worker responsibilities, and operational guardrails.

## 1. Web API design

The public-facing API is implemented as a gRPC service with accompanying REST/JSON exposure via [gRPC-Gateway](https://github.com/grpc-ecosystem/grpc-gateway). The protobuf definition lives in [`proto/web.proto`](../proto/web.proto).

### Service overview

| RPC | REST mapping | Purpose |
| --- | --- | --- |
| `CreateUploadURL` | `POST /v1/videos:uploadURL` | Issues a pre-signed S3 POST so the browser can upload the mezzanine asset directly. |
| `CompleteUpload` | `POST /v1/videos:completeUpload` | Idempotently records metadata and enqueues a transcoding job once the upload is confirmed. |
| `GetTranscodeStatus` | `GET /v1/videos/{video_id}/transcodeStatus` | Reports current transcoding progress and renditions produced so far. |
| `GetPlaybackInfo` | `GET /v1/videos/{video_id}/playbackInfo` | Returns DASH manifest locations along with streamable renditions. |
| `RecommendVideos` | `GET /v1/videos:recommendations` | Provides personalised video suggestions backed by the metadata index. |
| `SearchVideos` | `GET /v1/videos:search` | Full-text or tag search with cursor pagination. |

Each RPC carries precise HTTP bindings, response schemas, and pagination tokens so that the API can serve both the SPA and external partners. Authentication (not covered here) is enforced via gRPC interceptors and propagated through the gateway.

## 2. Upload pipeline

1. **Create upload session** – The web client calls `CreateUploadURL` with file metadata. The backend persists an `UploadSession` record (metadata + owner ID) in the Metadata service and returns a time-bound pre-signed POST URL + form fields.
2. **Browser direct upload** – The SPA performs a multipart/form-data POST to S3 using the credentials, streaming the video without touching application servers.
3. **Completion callback** – After `204 No Content`, the client (or S3 event bridge) invokes `CompleteUpload`. The service verifies the session, finalises metadata (`status=PENDING_TRANSCODE`), and publishes a job to the `transcode-jobs` queue (SQS/RabbitMQ).

Concurrency controls: upload sessions expire after 15 minutes, are idempotent, and require an `If-Match` token when updating metadata to prevent duplicate transcoding.

## 3. Transcoding worker

* Runs in a container image with `ffmpeg` + necessary codecs.
* Consumes messages from `transcode-jobs`; each payload references the mezzanine S3 key, desired renditions (1080p/720p/480p), and output prefix.
* Steps per job:
  1. Download or stream mezzanine from S3.
  2. Execute `ffmpeg` once, producing multiple H.264/MP4 fragments (`-f dash`) targeting 1080p, 720p, and 480p bitrates.
  3. Upload generated segments (`.m4s`) and the `manifest.mpd` to the output S3 bucket.
  4. Update metadata with rendition checksums, storage locations, and mark status `READY`.
  5. Emit metrics (`duration`, `cpu_seconds`, `retry_count`) to Prometheus and acknowledge the queue message.
* Failures trigger exponential backoff retries. Fatal errors mark the asset as `FAILED` so the UI can prompt a re-upload.

## 4. Web service & playback

The Web service composes data from Metadata, recommendation search indexes, and Storage manifests to build playback pages.

* `GetPlaybackInfo` returns manifest URL(s), subtitles, and DRM info consumed by the SPA.
* The SPA uses `dash.js` to request the MPD manifest and adaptively fetches segments from S3/CloudFront.
* Metadata service drives related content modules; recommendation service uses watch history + tags.
* All public assets are cached behind CDN; signed cookies or tokenised query params enforce viewer authorisation.

## 5. End-to-end load testing

* **Tooling**: `k6` (JavaScript) for HTTP + REST workload; Locust for queue and worker throughput validations.
* **Scenarios**:
  * Upload flood: sustain N concurrent uploads, ensure S3 presign latency < 100ms and queue latency < 500ms.
  * Transcode throughput: Locust generates queue load equal to target encodes/min, verifying worker autoscaling triggers before backlog > 2× steady state.
  * Playback fan-out: simulate 99th percentile traffic bursts hitting manifest + segment URLs.
* **SLOs**: 99.9% availability for API endpoints; encode job P95 completion within 5 minutes for 30-minute videos.
* **Scaling**:
  * Horizontal Pod Autoscalers on `transcode-worker` Deployments keyed on CPU/queue depth.
  * SQS queue redrive policies and DLQ for poison pills.
  * Observability via Grafana dashboards wired to Prometheus metrics and CloudWatch/S3 access logs.

This architecture unblocks parallel development across frontend, backend, and infrastructure teams while keeping the critical data flows explicit and testable.
