# hstongapi4go

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go" alt="Go">
  <img src="https://img.shields.io/badge/License-Apache%202.0-blue?style=flat-square" alt="Licencia">
  <img src="https://img.shields.io/badge/Status-alpha-blue?style=flat-square" alt="Estado">
  <img src="https://img.shields.io/badge/Gateway-v2.4.1-brightgreen?style=flat-square" alt="Versión de Gateway">
  <img src="https://img.shields.io/badge/Protobuf-v2.2.0-blueviolet?style=flat-square" alt="Versión del paquete Protobuf">
  <img src="https://img.shields.io/badge/Endpoints-51-orange?style=flat-square" alt="Endpoints HTTP">
  <img src="https://img.shields.io/badge/Push%20topics-11-yellowgreen?style=flat-square" alt="Temas de push de mercado">
  <a href="https://shing1211.github.io/hstongapi4go/"><img src="https://img.shields.io/badge/Docs-GitHub%20Pages-97CAFF?style=flat-square&logo=github" alt="Documentación"></a>
</p>

> **No oficial y alpha.** `hstongapi4go` es un SDK de Go de la comunidad para la
> **pasarela local** de HStong (华盛) Quant OpenAPI. **No está afiliado, autorizado,
> respaldado ni patrocinado por** HStong / 华盛 (Huasheng) ni ninguna de sus filiales.
> "HStong", "华盛", "华盛通" y los nombres y marcas relacionados son propiedad de sus
> respectivos dueños y se usan aquí solo con fines descriptivos y de interoperabilidad.
> Consulta [DISCLAIMER.md](./DISCLAIMER.md).

> **Nativo de Go. Con tipos. Gateway primero.** Un cliente Go idiomático para la
> pasarela local de HStong — datos de mercado, trading, futuros, algo y push en tiempo
> real — con importes y cantidades como cadenas y las mutaciones de órdenes nunca
> reintentadas automáticamente.

[English](./README.md) · [简体中文](./README.zh-Hans.md) · [繁體中文](./README.zh-Hant.md) · [日本語](./README.ja.md) · [한국어](./README.ko.md) · [Español](./README.es.md)

> Este documento es una traducción comunitaria del [README](./README.md) en inglés.
> **La versión en inglés es la autoritativa.** Sincronizado / Last synced: 2026-09-25

## Tabla de contenidos

- [Estado](#estado)
- [Instalación](#instalación)
- [Inicio rápido](#inicio-rápido)
- [Matriz de funciones](#matriz-de-funciones)
- [Configuración](#configuración)
- [Estructura de paquetes](#estructura-de-paquetes)
- [Documentación](#documentación)
- [Compilación y pruebas](#compilación-y-pruebas)
- [Contribuir](#contribuir)
- [Seguridad](#seguridad)
- [Licencia](#licencia)

---

## Estado

| Elemento | Estado |
|----------|--------|
| Núcleo del protocolo (envoltura HTTP, alias de rutas, códec híbrido, errores tipados) | Implementado |
| Datos de mercado pull (9 endpoints) | Implementado |
| Suscripción de mercado + push TCP (11 temas) | Implementado |
| Sesión de trading (login/logout, keep-alive, re-login single-flight) | Implementado |
| Activos / posiciones de trading (5) y órdenes (13) | Implementado |
| Suscripción push de trading + decodificadores de estado de órdenes | Implementado |
| Futuros (11 endpoints + push) | Implementado |
| Algo / estrategia (7 endpoints) | Implementado |
| API de streaming basada en canales (`Updates()` / `Errors()`) | Implementado |
| Mock Gateway (51 rutas HTTP + push TCP) y binario independiente | Implementado |
| Endurecimiento (rate limit, presupuesto de reintentos, circuit breaker, logging, métricas) | Implementado |
| Documentación (READMEs, sitio MkDocs, ADR, SPEC, LEGACY) | Implementado |
| Pruebas offline + e2e SDK-a-mock de todos los endpoints | Implementado |
| Pruebas de integración contra una pasarela real | Escritas y condicionadas por entorno; confirmación en vivo pendiente de ejecución del usuario |
| Publicación (GitHub + Gitee) | v0.1.14 ✓ |

Los 51 endpoints HTTP y los 11 temas de push de mercado están implementados. Los
recuentos son canónicos en [docs/SPEC.md](./docs/SPEC.md); no los edites a mano en
otro lugar.

## Instalación

```bash
go get github.com/shing1211/hstongapi4go
```

Requiere **Go 1.26+** y una [pasarela HStong OpenAPI](https://quant-open.hstong.com/api-docs/)
local en ejecución (o el [Mock Gateway](./docs/mock-gateway.md) incluido en el
repositorio). El SDK nunca instala, inicia ni redistribuye la pasarela; se conecta a
`http://127.0.0.1:11111` (HTTP) y `127.0.0.1:11112` (push TCP) por defecto.

## Inicio rápido

Construye el cliente una vez, compártelo y ciérralo al salir del programa. El cliente
es seguro para uso concurrente.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shing1211/hstongapi4go/pkg/hstong"
	"github.com/shing1211/hstongapi4go/pkg/hstong/market"
	"github.com/shing1211/hstongapi4go/pkg/hstong/trade"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Resuelve la configuración desde HSTONG_* (véase Configuración).
	c, err := client.New(client.WithEnv())
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	// 2. Inicia sesión en la sesión de trading (requiere HSTONG_TRADE_PASSWORD).
	session := hstong.NewSessionManager(c)
	if err := session.Login(ctx); err != nil {
		log.Fatalf("trade login: %v", err)
	}
	defer session.Logout(ctx)

	// 3. Solicita una cotización de mercado.
	marketMgr := market.New(c)
	quote, err := marketMgr.BasicQot(ctx, market.BasicQotRequest{
		Security: []*dto.Security{
			{DataType: int32(types.DataTypeHKStock), Code: "0700.HK"},
		},
		MktTmType: 1,
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, q := range quote.BasicQot {
		fmt.Printf("quote %s last=%v\n", q.GetSecurity().GetCode(), q.GetLastPrice())
	}

	// 4. Consulta las órdenes reales de hoy (requiere autenticación).
	tradeMgr := trade.New(c, trade.WithSession(session))
	orders, err := tradeMgr.RealEntrustList(ctx, trade.RealEntrustListRequest{
		ExchangeType: types.ExchangeHK,
		QueryCount:   20,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("today's real orders: %d\n", len(orders))
}
```

Suscríbete a un tema de push de mercado por el canal TCP (importa
`github.com/shing1211/hstongapi4go/pkg/hstong/stream`):

```go
s := stream.New(c)
if err := s.Connect(ctx); err != nil {
	log.Fatal(err)
}
defer s.Close()

sub, err := s.Subscribe(ctx, types.TopicBasicQot,
	&dto.Security{DataType: int32(types.DataTypeHKStock), Code: "0700.HK"})
if err != nil {
	log.Fatal(err)
}
defer sub.Cancel(context.Background())

for {
	select {
	case <-ctx.Done():
		return
	case ev := <-sub.Updates():
		if q, ok := ev.BasicQot(); ok {
			fmt.Printf("%s last=%v\n", ev.ID, q.GetBasicQot().GetLastPrice())
		}
	case err := <-sub.Errors():
		log.Printf("stream: %v", err)
	}
}
```

Hay programas ejecutables y compilables sin credenciales para cada superficie en
[`examples/`](./examples/README.md): `quickstart`, `market-data`, `trading`,
`futures`, `algo` y `streaming`.

## Matriz de funciones

Los recuentos de endpoints se toman solo de [docs/SPEC.md](./docs/SPEC.md). Todas las
rutas son `POST http://127.0.0.1:11111<route>`.

| Superficie | Paquete | Endpoints |
|---------|---------|-----------|
| Datos de mercado pull | `pkg/hstong/market` | 9 |
| Suscripción / cancelación de mercado | `pkg/hstong/market` | 2 |
| Sesión de trading | `pkg/hstong` | 2 |
| Activos / posiciones de trading | `pkg/hstong/trade` | 5 |
| Órdenes de trading | `pkg/hstong/trade` | 13 |
| Suscripción push de trading | `pkg/hstong/trade` | 2 |
| Algo / estrategia | `pkg/hstong/algo` | 7 |
| Futuros | `pkg/hstong/future` | 11 |
| **Total HTTP** | | **51** |

Temas de push de mercado (11): `11`, `35` (cotización); `14`, `27`, `28`, `37`
(ticks); `16` (cola de bróker); `17`, `25`, `26`, `36` (libro de órdenes). Consulta
[docs/SPEC.md §3](./docs/SPEC.md). El push de estado de órdenes de trading y futuros
usa la familia `TradeStockDeliverNotify` en el mismo canal TCP.

El códec híbrido es **por endpoint**: los cuerpos HTTP de mercado se decodifican con
`encoding/json` sobre DTO tipados, y los payloads de push TCP con protobuf binario;
los cuerpos de trading, futuros, algo, activos y sesión son structs JSON escritos a
mano. Los importes y cantidades son `string` (o `json.Number`), nunca `float64`. Las
mutaciones de órdenes, futuros y algo emiten exactamente un intento y nunca se
reintentan automáticamente.

## Configuración

`client.New` aplica opciones funcionales en orden sobre los valores por defecto
documentados. Cada variable `HSTONG_*` de abajo la lee `client.WithEnv()`; una opción
explícita colocada después de `WithEnv` anula el entorno.

| Variable | Formato | Valor por defecto | Propósito |
|----------|--------|---------|---------|
| `HSTONG_GATEWAY_URL` | URL absoluta `http`/`https` | `http://127.0.0.1:11111` | Raíz HTTP de la pasarela |
| `HSTONG_PUSH_ADDR` | `host:port` | `127.0.0.1:11112` | Dirección de push TCP de la pasarela |
| `HSTONG_TIMEOUT` | duración Go (`10s`, `1500ms`) | `10s` | Timeout por petición y `timeout_sec` de la envoltura |
| `HSTONG_TRADE_PASSWORD` | texto plano | sin definir | Contraseña de trading; se cifra antes de `TradeLogin`, nunca se registra |
| `HSTONG_VERIFY_PUSH` | `strconv.ParseBool` | `false` | Activa opcionalmente la verificación `SHA1WithRSA` de las tramas push |

Las pruebas de integración contra una pasarela real se omiten por defecto y usan sus
propias variables (consulta [`test/integration/README.md`](./test/integration/README.md)):

| Variable | Obligatoria | Valor por defecto | Propósito |
|----------|----------|---------|---------|
| `HSTONG_INTEGRATION` | sí (puerta) | — | Debe ser `1`; de lo contrario se omiten todas las pruebas |
| `HSTONG_GATEWAY_URL` | no | `http://127.0.0.1:11111` | Raíz HTTP de la pasarela |
| `HSTONG_PUSH_ADDR` | no | `127.0.0.1:11112` | Dirección de push TCP de la pasarela |
| `HSTONG_TRADE_PASSWORD` | para pruebas de sesión/trading | — | Contraseña de trading en texto plano |
| `HSTONG_TEST_SYMBOL` | no | `00700.HK` | Instrumento bajo prueba |
| `HSTONG_TEST_EXCHANGE` | no | `K` | `K` Hong Kong, `P` EE. UU., `v` Shenzhen, `t` Shanghái |
| `HSTONG_VERIFY_PUSH` | no | desactivado | Activa la verificación de firma push |
| `HSTONG_PLACE_ORDERS` | no | desactivado | Debe ser `1` para habilitar la prueba de mutación de órdenes |

## Estructura de paquetes

```
hstongapi4go/
├── client/            # Cliente principal: opciones, entorno, rutas, códecs, Close
├── pkg/hstong/        # Gestores públicos: sesión + market/trade/future/algo/stream
├── pkg/types/         # Enumeraciones, códigos de estado, claves públicas de la plataforma
├── pkg/domain/        # v-next: decimal value types, typed IDs, DTO mappers
├── pkg/services/      # v-next: market/account/trading use cases
├── pkg/transport/     # v-next: HTTP adapter (deadlines, caps, retry)
├── internal/          # Privado: transport, push, crypto, errs, resilience, logging, metrics
├── gen/               # Código protobuf generado (NO EDITAR)
├── proto/             # Fuentes .proto incluidas + procedencia
├── test/mockgateway/  # Servidor mock HTTP + push TCP offline
├── test/e2e/          # Pruebas SDK-a-mock de todos los endpoints
├── test/integration/  # Pruebas reales condicionadas por entorno (omitidas por defecto)
├── cmd/               # Binarios independientes (hstong-mock-gateway)
├── examples/          # Ejemplos ejecutables (compatibles con mock)
├── scripts/           # Scripts de compilación y verificación
└── docs/              # Sitio MkDocs, SPEC, ADR, DESIGN, LEGACY
```

## Documentación

- Sitio de documentación: <https://shing1211.github.io/hstongapi4go/>
- Índice canónico de la API y recuentos: [docs/SPEC.md](./docs/SPEC.md)
- Arquitectura: [ARCHITECTURE.md](./ARCHITECTURE.md) · [docs/DESIGN.md](./docs/DESIGN.md)
- Decisiones: [docs/adr/README.md](./docs/adr/README.md) (ADR 0001–0011)
- Protocolo heredado (documentado, **no implementado**): [docs/LEGACY.md](./docs/LEGACY.md)
- Aviso legal: [DISCLAIMER.md](./DISCLAIMER.md)

El **protocolo heredado directo a la plataforma** — `/hs/v2/login`,
`/hs/config/queryServer`, el socket directo de 151 bytes con firma RSA del
desarrollador, claves dinámicas AES-ECB, heartbeat y vinculación de dispositivo — se
documenta solo como referencia y **no está implementado** en este SDK. Consulta
[docs/LEGACY.md](./docs/LEGACY.md).

## Compilación y pruebas

```bash
make help            # lista los objetivos
make build           # go build ./...
make check           # fmt + vet + money-check + pruebas unitarias
make test-race       # go test -race -count=1 ./...
make test-integration  # pruebas reales condicionadas por entorno (requieren HSTONG_*)
make proto-verify    # falla si gen/ se desvía de proto/
make docs-check      # comprobación de enlaces markdown + mkdocs build --strict
make mock-gateway    # ejecuta el Mock Gateway independiente
```

Comandos directos que deben pasar sin credenciales:

```bash
go build ./...
go vet ./...
gofmt -l .            # no debe imprimir nada
go test ./...
go test -race -count=1 ./...
```

Las pruebas unitarias son offline y no necesitan credenciales. Las pruebas de
integración son las únicas que pueden requerir red o credenciales.

## Contribuir

Consulta [CONTRIBUTING.md](./CONTRIBUTING.md). Todas las confirmaciones deben estar
firmadas con DCO (`git commit -s`) y seguir Conventional Commits referenciando los IDs
de run y tarea. Las nuevas dependencias de ejecución requieren un ADR; nunca edites el
código generado en `gen/`; nunca reintentes automáticamente mutaciones de órdenes.

## Seguridad

Consulta [SECURITY.md](./SECURITY.md). El SDK no guarda credenciales de la plataforma
ni la clave privada del desarrollador; las claves públicas de la plataforma incluidas
son datos de referencia públicos. El único valor sensible que maneja el SDK es la
contraseña de trading en texto plano, que cifra en tránsito y nunca registra.

## Licencia

[Apache License 2.0](./LICENSE). Consulta también [NOTICE](./NOTICE),
[DISCLAIMER.md](./DISCLAIMER.md) y [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md).
