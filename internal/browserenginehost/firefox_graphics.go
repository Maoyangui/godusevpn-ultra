package browserenginehost

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"time"
)

func (d *firefoxDriver) configureGraphics(ctx context.Context, ua string) error {
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return ErrGraphicsReadback
	}
	// A different fixed HTTPS origin is fulfilled entirely in memory. The
	// session relay remains closed until this and the existing UA probes pass.
	cross := "https://godusevpn-graphics.invalid/owned/" + hex.EncodeToString(nonce[:])
	observed := make(chan error, 1)
	d.mu.Lock()
	d.graphicsURL, d.graphicsUA, d.graphicsObserved = cross, ua, observed
	id := d.contextID
	d.mu.Unlock()
	defer func() { d.mu.Lock(); d.graphicsURL, d.graphicsUA, d.graphicsObserved = "", "", nil; d.mu.Unlock() }()
	raw, err := d.call(ctx, "script.evaluate", map[string]any{"expression": graphicsProbeExpression(d.config, cross), "target": map[string]string{"context": id}, "awaitPromise": true, "resultOwnership": "none"})
	if err != nil {
		return ErrGraphicsReadback
	}
	value, err := firefoxEvaluation(raw)
	if err != nil {
		return ErrGraphicsReadback
	}
	evidence, err := verifyGraphicsReadback(value, d.config)
	if err != nil {
		return err
	}
	select {
	case err := <-observed:
		if err != nil {
			return ErrGraphicsReadback
		}
	case <-ctx.Done():
		return ErrGraphicsReadback
	case <-d.client.done:
		return ErrGraphicsReadback
	case <-time.After(5 * time.Second):
		return ErrGraphicsReadback
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || ctx.Err() != nil {
		return ErrGraphicsReadback
	}
	d.graphicsEvidence = evidence
	return nil
}

func (d *firefoxDriver) fulfillGraphicsFrame(ctx context.Context, request string) error {
	header := func(name, value string) map[string]any {
		return map[string]any{"name": name, "value": map[string]string{"type": "string", "value": value}}
	}
	_, err := d.call(ctx, "network.provideResponse", map[string]any{"request": request, "statusCode": 200, "headers": []any{header("Content-Type", "text/html; charset=utf-8"), header("Cache-Control", "no-store"), header("Content-Security-Policy", workerCSP)}, "body": map[string]string{"type": "base64", "value": base64.StdEncoding.EncodeToString([]byte(graphicsFrameHTML(d.config)))}})
	return err
}

func (d *firefoxDriver) GraphicsEvidence() *GraphicsEvidence {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.graphicsEvidence == nil {
		return nil
	}
	copy := *d.graphicsEvidence
	return &copy
}
