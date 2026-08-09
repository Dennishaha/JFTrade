// Package servercore is the API sidecar composition root.
//
// It wires transport handlers, domain services, broker integrations, stores,
// background workers and live publishers. Business policy belongs in the
// internal domain packages; HTTP parsing and response contracts belong in
// internal/api packages.
//
// Keep this package focused on process assembly and lifecycle. New behavior
// should normally be added to the owning service or one of the narrow app
// handles under internal/app/apiserver, then passed into Server through an
// interface. Do not use this comment as a file index: file names are allowed
// to change as responsibilities are extracted.
package servercore
