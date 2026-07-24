# Scriberr "on the go" — deploy gratis (Hugging Face Space + Groq)

Objetivo: Scriberr **accesible desde cualquier lado**, **$0**, e **instalable como
app en el celular** (es una PWA). Sin GPU, sin 15GB de modelos: transcribe con la
API gratuita de **Groq** (Whisper large v3), compatible con el protocolo de OpenAI.

> Aplica cuando el audio **no es confidencial** (pasa por Groq). Para privacidad
> total, ver el final (VPS con modelos locales); la misma PWA sirve encima.

## Arquitectura $0

```
Celular (PWA)  ──▶  Scriberr en HF Space (gratis, Docker)  ──▶  Groq API (gratis)
                        UI + backend Go liviano                 Whisper large v3
```

- **Hosting**: Hugging Face Space (tier gratis, CPU) — el trabajo pesado se va a
  Groq, así que la CPU del Space no importa.
- **Transcripción**: Groq free tier, rápido, sin costo.
- **Imagen**: `Dockerfile.lite` — sin `uv`/python/modelos. Al arrancar, los modelos
  locales fallan (solo warning) y queda activo `openai_whisper`, apuntado a Groq.

---

---

## Opción recomendada — Render (desde GitHub, casi un clic)

Render despliega **directo desde este repo** (no hay `git push` manual a ningún
remoto). Ya viene un `render.yaml` (Blueprint) listo.

1. **Key de Groq**: [console.groq.com](https://console.groq.com) → API Keys →
   Create API Key → copia `gsk_...`.
2. [render.com](https://render.com) → regístrate con GitHub (gratis).
3. **New → Blueprint** → conecta tu repo `Scriberr` → Render detecta `render.yaml`
   → **Apply**.
4. En el servicio, **Environment → Add** el secret `OPENAI_API_KEY` = tu key de
   Groq (las otras variables ya vienen en el blueprint). Save → redeploy.
5. Cuando el build termina, Render te da una **URL HTTPS pública**. Ábrela →
   regístrate → sube audio → modelo **"OpenAI Whisper"**, campo *model* =
   `whisper-large-v3`, idioma `es`.
6. **Instala la PWA**: iPhone (Safari) → Compartir → "Agregar a pantalla de
   inicio"; Android (Chrome) → ⋮ → "Instalar aplicación".

> Free tier de Render: el servicio se **duerme tras ~15 min** sin uso (arranque
> frío ~1 min al abrirlo) y su disco es **efímero** (la base se reinicia en
> redeploys — te re-registras). Igual que HF, el trabajo pesado va a Groq.

---

## Alternativa — Hugging Face Space

## Paso 1 — Key gratis de Groq

1. Entra a [console.groq.com](https://console.groq.com) (registro gratis).
2. **API Keys → Create API Key**. Cópiala (`gsk_...`).

## Paso 2 — Crear el Space

1. En [huggingface.co](https://huggingface.co) → **New → Space**.
2. **Space SDK: Docker** (blank). Nombre a gusto. Visibilidad: Public o Private.

## Paso 3 — Subir el código al Space

El Space se construye desde su propio repo git. Desde tu clon de Scriberr:

```bash
# 1) el Space usa el Dockerfile liviano como Dockerfile raíz
cp Dockerfile.lite Dockerfile

# 2) el README del Space necesita el frontmatter YAML de Hugging Face
cp deploy/hf-space/README.hf.md README.md

git add Dockerfile README.md
git commit -m "HF Space: imagen liviana + config"

# 3) empuja al remoto del Space (usa tu usuario/space y token HF como password)
git remote add space https://huggingface.co/spaces/<TU_USUARIO>/<TU_SPACE>
git push space HEAD:main
```

> El `cp Dockerfile.lite Dockerfile` y el README con frontmatter solo importan en la
> rama que empujas al Space. En tu repo normal de GitHub no cambies el `Dockerfile`
> original ni el `README.md`.

## Paso 4 — Secrets del Space

En el Space → **Settings → Variables and secrets → New secret**:

| Nombre | Valor |
|---|---|
| `OPENAI_API_KEY` | tu key de Groq (`gsk_...`) |
| `OPENAI_BASE_URL` | `https://api.groq.com/openai/v1` |
| `JWT_SECRET` | `openssl rand -hex 32` |

El Space reconstruye solo. Cuando el build termina, abre la URL del Space.

## Paso 5 — Primer uso

1. Abre la URL → **regístrate** (el primer usuario queda como admin).
2. Sube un audio → en la config de transcripción elige el modelo **"OpenAI Whisper"**,
   y en el campo *model* pon **`whisper-large-v3`** (es el de Groq). Idioma: `es`.
   > Los modelos locales aparecerán en error: es lo esperado en esta imagen.

## Paso 6 — Instalar como app en el celular (PWA)

- **iPhone (Safari):** Compartir → **"Agregar a pantalla de inicio"**.
- **Android (Chrome):** menú ⋮ → **"Instalar aplicación"**.

Queda como app nativa, pantalla completa, desde cualquier red.

---

## Caveats honestos ($0 tiene trade-offs)

- **Almacenamiento efímero** en el HF Space gratis: si el Space se reconstruye,
  se pierden la base (usuarios, historial) y los audios. Para uso personal es
  tolerable (te re-registras). Persistencia durable = storage de pago del Space
  (~USD 5/mes) o exportar periódicamente.
- **El Space "duerme"** tras 48h sin visitas; despierta solo al abrir la URL
  (arranque frío de ~30-60s).
- **Límite ~25MB por archivo** (Groq free). Una hora de comité puede excederlo;
  comprime a mp3 mono ~64kbps o trocea el audio.
- **Diarización por hablante**: la ruta cloud no la trae (necesita modelos
  locales). Da texto + timestamps de segmento; para separar hablantes evalúa un
  modelo `*-diarize` si el proveedor lo soporta.
- El audio **sale hacia Groq** — solo material no confidencial.

## Alternativa privada (no gratis, sin terceros)

Si algún día el audio es confidencial: VPS/GPU con la imagen completa
(`Dockerfile` + `docker-compose.cuda.yml`), modelos locales, HTTPS + login. Misma
PWA instalable encima. La decisión "cloud vs local" es la Fase 0 del plan de minutas.

## Otros hosts compatibles con `Dockerfile.lite`

Fly.io (`deploy/fly.toml`), Render, Koyeb o cualquier VPS con Docker sirven la
misma imagen liviana; setea `OPENAI_API_KEY`, `OPENAI_BASE_URL` y un volumen en
`/app/data` para persistir.
