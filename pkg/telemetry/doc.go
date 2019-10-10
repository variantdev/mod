// telemetry provides traces and metrics for various spans, and contextual, leveled logging.
// Supported metrics includes:
// - rps(*_started_total)
// - success/error count(*_handled_total)
// - latency histogram(*_handling_seconds_bucket)
//
// See example/main.go for usage example
package telemetry
