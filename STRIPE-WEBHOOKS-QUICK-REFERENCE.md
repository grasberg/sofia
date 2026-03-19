# Stripe Webhook Best Practices - Quick Reference

## 🚨 Critical Requirements
1. **Always verify signatures** - Use `Stripe-Signature` header with raw request body
2. **HTTPS required** - TLS 1.2+ with valid certificate in production
3. **Quick 2xx response** - Respond within 1-2 seconds, process asynchronously
4. **Idempotent handlers** - Handle duplicate events safely

## 🔐 Security Essentials
- **Signature verification**: `stripe.webhooks.constructEvent(payload, signature, secret)`
- **Replay protection**: Check timestamp < 5 minutes old
- **IP allowlisting**: Optional additional layer (use Stripe's published IPs)
- **Secret rotation**: Rotate webhook secrets every 90 days

## ⚡ Performance Best Practices
- **Queue processing**: Don't process in webhook handler thread
- **No order guarantees**: Events may arrive out of order
- **Retry handling**: Stripe retries for 3 days with exponential backoff
- **Error monitoring**: Track 4xx/5xx responses in Stripe Dashboard

## 🛠️ Implementation Checklist
- [ ] Use Stripe SDK for signature verification
- [ ] Access raw request body before JSON parsing
- [ ] Validate timestamp (5-minute tolerance)
- [ ] Implement idempotency (track processed event IDs)
- [ ] Exempt webhook route from CSRF protection
- [ ] Test with Stripe CLI locally
- [ ] Monitor delivery failures

## 📚 Full Documentation
See: `docs/webhooks/stripe/best-practices-2026.md` for complete guide with code examples, troubleshooting, and advanced patterns.

---

**Common Pitfalls:**
1. **Signature fails** → Framework parsed JSON before verification. Use `express.raw()` for webhook route.
2. **Timeout errors** → Processing takes >10 seconds. Queue and respond immediately.
3. **Duplicate events** → Implement idempotency keys or event ID tracking.
4. **Order dependencies** → Fetch current state from API, don't rely on event sequence.

**Testing:**
```bash
stripe listen --forward-to localhost:3000/webhook
stripe trigger checkout.session.completed
```

**Last Updated:** March 2026