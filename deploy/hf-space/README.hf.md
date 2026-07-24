---
title: Scriberr
emoji: 🎙️
colorFrom: blue
colorTo: indigo
sdk: docker
app_port: 8080
pinned: false
---

# Scriberr (on the go)

Transcripción de audio como PWA, hospedada gratis en un Hugging Face Space.
Transcribe vía un endpoint compatible con OpenAI (Groq, free tier) — sin GPU
ni modelos locales.

**Este README es el que va en el repo del Space** (el bloque YAML de arriba es
obligatorio para que Hugging Face configure el Space como Docker en el puerto 8080).

## Secrets a configurar en el Space (Settings → Variables and secrets)

- `OPENAI_API_KEY` — tu key gratis de Groq (console.groq.com).
- `OPENAI_BASE_URL` — `https://api.groq.com/openai/v1`
- `JWT_SECRET` — cadena aleatoria larga (`openssl rand -hex 32`).

Ver la guía completa en `DEPLOY-ON-THE-GO.md`.
