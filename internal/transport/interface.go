package transport

import "github.com/goppydae/gapi/internal/eventbus"

// Alias so code that imports internal/transport can refer to Transport[T],
// while the canonical interface lives in eventbus (preventing import cycles).
type Transport[T any] = eventbus.Transport[T]
