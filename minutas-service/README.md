# minutas-service

Capa de **síntesis y entrega** de minutas/actas de reunión sobre transcripciones
producidas por **Scriberr** (motor de transcripción headless).

```
audio ──▶ Scriberr (transcribe + diariza) ──▶ minutas-service (sintetiza + PDF) ──▶ entrega
```

**Principio:** el producto es agnóstico de cliente. Ningún nombre de industria o
cliente aparece en el código. El contexto sectorial (glosario, siglas, roles,
formato) vive en `profiles/<x>.yaml`, se carga en runtime y se inyecta en el
prompt de síntesis. Agregar un cliente es escribir un YAML — no tocar el repo.

Antes de trabajar en este repo, lee **[`CLAUDE.md`](./CLAUDE.md)**: fija la regla
de neutralidad, la frontera con el motor, el mapa de fases (una fase por sesión)
y la auditoría de neutralidad.

## Estado

Fase de setup (PARTE 1): governance y frontera de neutralidad establecidas.
El código de cada fase se construye en sesiones separadas — ver `CLAUDE.md §6`.
