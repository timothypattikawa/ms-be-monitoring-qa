# Knowledge & RAG Monitoring — rancangan

Menu baru di dashboard (setelah "Documentation Matrix"). Memantau Solr (vector DB) dan status sinkronisasi dokumen RAG.
Referensi UI: screenshot PulseQA tab "Knowledge & RAG". Solr: `http://10.4.252.13:8983/solr` (VPN only).

## Env (`.env`)
| Key | Fungsi |
|---|---|
| `SOLR_BASE_URL`, `SOLR_USER/PASSWORD`, `SOLR_TIMEOUT` | koneksi (basic auth opsional) |
| `SOLR_VECTOR_COLLECTION` | core vector utama (`testcase_vectors`, 37.942 docs) |
| `SOLR_COLLECTION_PREFIX` | collection yang ditampilkan: semua `tc_*` (terlihat: tc_apo, tc_apomitra, tc_backoffice, tc_eservice, tc_marketplace, tc_mylawson, tc_payment, tc_promo, tc_voucher; daftar diambil dinamis dari Solr, bukan hardcode); `testcase_vectors` dikecualikan dari grid |
| `SOLR_EMBEDDING_MODEL`, `SOLR_VECTOR_DIMENSION` | info model (Solr tidak menyimpan nama model, hanya dimensi) |
| `SOLR_COLLECTION_TARGETS` | `collection=totalTarget` untuk Coverage %; opsional, kosong → coverage tampil "-" |
| `SOLR_SYNC_STALE_AFTER` | lewat ini → `OUTDATED_SYNC` |

## Sumber data Solr (read-only)
| Kebutuhan UI | Endpoint Solr |
|---|---|
| RAM / Heap | `GET /admin/info/system` → `jvm.memory.raw.used/max` (heap) dan `system.totalPhysicalMemorySize/freePhysicalMemorySize` |
| Daftar collection + doc count + last sync | `GET /admin/cores?action=STATUS` → per core `index.numDocs`, `index.lastModified`, `index.sizeInBytes` (core terlihat standalone di Solr Admin) |
| Vector dimension | `GET /{core}/schema/fieldtypes` → `solr.DenseVectorField.vectorDimension` |
| Tabel dokumen | `GET /{core}/select?q=*:*&fl=id,title,source,project,suite_id&sort=...&start&rows` (+ `q=title:*kata*` untuk search) |

Last sync = `index.lastModified` (proxy: waktu index terakhir berubah). Tidak ada field "synced_at" di dokumen sekarang.

## API backend (`/api/v1/knowledge`, pola sama `documents`)
- `GET /knowledge/overview` → `{ ram:{usedGb,totalGb,pct}, lastFullSync, embedding:{model,dimension}, collectionsActive, autoSyncEvery }`
- `GET /knowledge/collections` → `[{ name, docCount, target, coveragePct, status, lastSyncedAt, outdatedDocs, sizeBytes }]`
- `GET /knowledge/projects` → `{ totals:{collections,docs,projects,emptyCollections}, collections:[{ name,label,docCount,projectCount,projects:[{code,name,docs}] }] }` (untuk UI dashboard baru: kartu ringkasan, bar chart, section per collection). Endpoint `collections` dan `documents` di atas tetap ada.
- `GET /knowledge/documents?collection=&q=&page=&pageSize=` → `{ items:[{ id, title, key, sourceUrl, collection, chunks, dims, lastSyncedAt, syncStatus }], total }`
- `POST /knowledge/collections/:name/sync` | `/reindex` (managerAuth) → membuat sync job, dipoll lewat `/sync-jobs/:id`. **Perlu keputusan**: pipeline embedding/indexing ada di luar repo ini — endpoint ini hanya trigger (webhook/queue?) ke pipeline tsb.

Status collection (dihitung backend):
- `HEALTHY`: coverage ≥ 90% dan sync < `SOLR_SYNC_STALE_AFTER`
- `OUTDATED_SYNC`: sync lewat batas stale
- `NEEDS_REINDEX`: coverage < 70% atau docCount = 0
Status dokumen: `SYNCED`, `QUEUED`, `FAILED` (mis. token exceeded), `SYNCING`.

## Gap data (tidak bisa dari Solr saja)
1. **Target total** → env `SOLR_COLLECTION_TARGETS` dulu; lanjut: hitung dari Qase (jumlah case per project) jika mapping collection→project jelas.
2. **Chunks per dokumen**: skema sekarang 1 doc = 1 vector (`_root_` = id) → chunks = 1. Kolom ini baru bermakna kalau ada field parent/chunk.
3. **Status QUEUED/FAILED & "Token Exceeded"**: hanya diketahui pipeline indexing → butuh tabel `knowledge_sync_log` (Postgres) yang ditulis pipeline/endpoint sync.
4. **Sync/Reindex** → lihat keputusan di atas.

## Urutan kerja
1. BE: `internal/monitoring/knowledge_http.go` + client Solr kecil (net/http, mengikuti `Connector`) (`internal/monitoring/solr.go`), tanpa mock (test pakai httptest fake Solr).
2. FE (`monitoring-qa-alfagift`): page `pages/knowledge` + tab di nav, 3 blok sesuai screenshot (header metrics, grid collection, tabel dokumen).
3. Deploy di jaringan yang bisa jangkau Solr.

## Keputusan implementasi
- Respons .send`), sama seperti `/documents`.
- `lastFullSync` = `lastModified` terbaru di antara collection. `collectionsActive` = jumlah collection dengan docCount > 0.
- Status: `NEEDS_REINDEX` (docCount=0 atau coverage<70) > `OUTDATED_SYNC` (lastModified lebih tua dari stale-after / tidak ada) > `HEALTHY`. Coverage 70-90% tidak stale dianggap `HEALTHY`.
- `/knowledge/documents` tanpa `collection` -> query vector collection; `collection` tiap item = `prefix+lower(project)` bila core itu ada, selain itu nama vector collection. `collection` divalidasi terhadap daftar core dinamis (404 `COLLECTION_NOT_FOUND`).
- Solr tidak terjangkau -> 502 `SOLR_UNAVAILABLE` (tanpa URL/kredensial). `SOLR_SYNC_STALE_AFTER` mendukung sufiks `d`.
- Sync/reindex: validasi nama collection, POST `{collection,mode}` ke `SOLR_SYNC_WEBHOOK_URL`; respons 202 `{collection,mode,status:"ACCEPTED"}`.
- `/knowledge/projects`: collection = vector collection (label "Default / Utama", pertama) + core prefix (urut nama), termasuk yang kosong. Jumlah per project dari facet Solr `project` (maks 4 request paralel, timeout 15s per core); core gagal -> `projects:[]` + `error:"facet_failed"`, STATUS gagal -> 502 `SOLR_UNAVAILABLE`. `totals.projects` = kode project distinct lintas collection. Nama project dari Qase `GET /v1/project` (cache memori 10 menit), fallback tabel `projects` lokal, lalu `""`.
