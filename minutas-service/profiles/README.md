# profiles/

**La única frontera del repo donde puede aparecer contexto de cliente.**

Un perfil de cliente (`<x>.yaml`) aporta, en runtime, el contexto sectorial que
el código no conoce: glosario, siglas, roles, formato de acta, tono. Se **carga
e inyecta en el prompt de síntesis** según el job (Fase 1b). El código solo sabe
de "un perfil"; nunca de *cuál*.

## Reglas

1. **Los perfiles reales no se commitean.** El `.gitignore` ignora `*.yaml` /
   `*.yml` salvo `_example.*`. Se montan/cargan en runtime.
2. **Agregar un cliente = escribir un YAML.** No se toca el repo. Si el YAML no
   tiene dónde poner algo, el arreglo es **ampliar el schema del perfil**
   (campo genérico, sin nombre de cliente), no ramificar el código.
3. **De dónde salen los YAML:** en una **sesión aparte** se carga el skill
   sectorial correspondiente (`banca-chilena`, `isapres-chile-experto`,
   `sii-chile-experto`) y se redacta el perfil. Ese skill **nunca** entra en una
   sesión de build.

## Schema

El schema se congela en **Fase 1 / 1b**, informado por `decision-logger`. Hasta
entonces, `_example.yaml` es ilustrativo y **no normativo**.
