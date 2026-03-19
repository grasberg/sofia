/**
 * Authentication utilities for API endpoints
 * Simple API key authentication for affiliate API access
 */

import { NextRequest } from 'next/server'

export interface AuthResult {
  authenticated: boolean
  affiliateId?: string
  error?: string
  status?: number
}

/**
 * Verify API key from request headers
 * Expected header: X-API-Key: <api-key>
 * API keys are stored in environment variables as AFFILIATE_API_KEYS
 * Format: affiliateId:apiKey,affiliateId2:apiKey2
 */
export function verifyApiKey(request: NextRequest): AuthResult {
  const apiKeyHeader = request.headers.get('X-API-Key')
  
  if (!apiKeyHeader) {
    return {
      authenticated: false,
      error: 'Missing API key',
      status: 401
    }
  }

  const apiKeysEnv = process.env.AFFILIATE_API_KEYS
  if (!apiKeysEnv) {
    // If no API keys configured, allow any key (for development only)
    console.warn('WARNING: AFFILIATE_API_KEYS environment variable not set. Allowing all requests.')
    return {
      authenticated: true,
      affiliateId: 'dev'
    }
  }

  // Parse API keys from environment variable
  const keyPairs = apiKeysEnv.split(',')
    .map(pair => pair.trim())
    .filter(Boolean)
    .map(pair => {
      const [affiliateId, key] = pair.split(':')
      return { affiliateId: affiliateId?.trim(), key: key?.trim() }
    })
    .filter(pair => pair.affiliateId && pair.key)

  // Find matching key
  const match = keyPairs.find(pair => pair.key === apiKeyHeader)
  if (!match) {
    return {
      authenticated: false,
      error: 'Invalid API key',
      status: 401
    }
  }

  return {
    authenticated: true,
    affiliateId: match.affiliateId
  }
}

/**
 * Middleware wrapper for API routes requiring authentication
 * Returns the authenticated affiliate ID or throws an error response
 */
export function withAuth(handler: (req: NextRequest, affiliateId: string) => Promise<Response>) {
  return async (request: NextRequest) => {
    const authResult = verifyApiKey(request)
    
    if (!authResult.authenticated) {
      return new Response(
        JSON.stringify({ error: authResult.error || 'Unauthorized' }),
        {
          status: authResult.status || 401,
          headers: { 'Content-Type': 'application/json' }
        }
      )
    }

    return handler(request, authResult.affiliateId!)
  }
}

/**
 * Helper to extract bearer token from Authorization header
 * Format: Bearer <token>
 */
export function getBearerToken(request: NextRequest): string | null {
  const authHeader = request.headers.get('Authorization')
  if (!authHeader || !authHeader.startsWith('Bearer ')) {
    return null
  }
  return authHeader.substring(7)
}

/**
 * Verify JWT token (placeholder - implement actual JWT verification if needed)
 */
export function verifyJwt(token: string): AuthResult {
  // TODO: Implement JWT verification with secret
  // For now, just a placeholder
  return {
    authenticated: false,
    error: 'JWT authentication not implemented',
    status: 501
  }
}