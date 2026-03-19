/**
 * API: /api/affiliate/links
 * Manage affiliate tracking links
 */

import { NextRequest, NextResponse } from 'next/server'
import { affiliateLinkGenerator, type CreateAffiliateLinkInput, type AffiliateLink } from '@/lib/affiliate-link-generator'
import { affiliateService } from '@/lib/AffiliateService'
import { verifyApiKey } from '@/lib/auth'

// In-memory store for links (replace with database in production)
const links: Map<string, AffiliateLink> = new Map()
const linksByAffiliate: Map<string, Set<string>> = new Map()
const shortCodeIndex: Map<string, string> = new Map() // shortCode -> linkId

// Helper to authenticate and get affiliate ID
function authenticateRequest(request: NextRequest): { success: true; affiliateId: string } | { success: false; response: NextResponse } {
  const authResult = verifyApiKey(request)
  if (!authResult.authenticated) {
    return {
      success: false,
      response: NextResponse.json(
        { error: authResult.error || 'Unauthorized' },
        { status: authResult.status || 401 }
      )
    }
  }
  return { success: true, affiliateId: authResult.affiliateId! }
}

// Helper to verify affiliate owns the link
function verifyLinkOwnership(linkId: string, affiliateId: string): { success: true; link: AffiliateLink } | { success: false; response: NextResponse } {
  const link = links.get(linkId)
  if (!link) {
    return {
      success: false,
      response: NextResponse.json(
        { error: 'Link not found' },
        { status: 404 }
      )
    }
  }
  if (link.affiliateId !== affiliateId) {
    return {
      success: false,
      response: NextResponse.json(
        { error: 'Access denied. Link does not belong to you.' },
        { status: 403 }
      )
    }
  }
  return { success: true, link }
}

export async function GET(request: NextRequest) {
  try {
    const auth = authenticateRequest(request)
    if (!auth.success) return auth.response
    const { affiliateId } = auth

    const { searchParams } = new URL(request.url)
    const queryAffiliateId = searchParams.get('affiliateId')
    const linkId = searchParams.get('linkId')
    const status = searchParams.get('status')

    // If affiliate ID is provided in query params, verify it matches authenticated affiliate
    if (queryAffiliateId && queryAffiliateId !== affiliateId) {
      return NextResponse.json(
        { error: 'Access denied. You can only access your own links.' },
        { status: 403 }
      )
    }

    // Use authenticated affiliate ID if no affiliateId provided
    const targetAffiliateId = queryAffiliateId || affiliateId

    // Get single link by ID
    if (linkId) {
      const verification = verifyLinkOwnership(linkId, targetAffiliateId)
      if (!verification.success) return verification.response
      return NextResponse.json({ link: verification.link })
    }

    // Get links by affiliate
    let result: AffiliateLink[] = []
    
    if (targetAffiliateId) {
      const linkIds = linksByAffiliate.get(targetAffiliateId) || new Set()
      result = Array.from(linkIds).map(id => links.get(id)).filter(Boolean) as AffiliateLink[]
    } else {
      // Admin can see all links (if no affiliate ID and authenticated as admin)
      if (affiliateId === 'admin') {
        result = Array.from(links.values())
      } else {
        return NextResponse.json(
          { error: 'Access denied. Admin only.' },
          { status: 403 }
        )
      }
    }

    // Filter by status
    if (status) {
      result = result.filter(link => link.status === status)
    }

    // Sort by createdAt descending
    result.sort((a, b) => b.createdAt.getTime() - a.createdAt.getTime())

    return NextResponse.json({ 
      links: result,
      total: result.length,
    })
  } catch (error) {
    console.error('GET links error:', error)
    return NextResponse.json({ error: 'Failed to fetch links' }, { status: 500 })
  }
}

export async function POST(request: NextRequest) {
  try {
    const auth = authenticateRequest(request)
    if (!auth.success) return auth.response
    const { affiliateId: authenticatedAffiliateId } = auth

    const body = await request.json()
    const { 
      affiliateId,
      affiliateCode,
      type,
      targetUrl,
      utmCampaign,
      utmMedium,
      utmContent,
      utmTerm,
      expiresAt,
      metadata,
    } = body

    // Validate required fields
    if (!affiliateId || !affiliateCode || !targetUrl) {
      return NextResponse.json(
        { error: 'affiliateId, affiliateCode, and targetUrl are required' },
        { status: 400 }
      )
    }

    // Verify affiliate ID matches authenticated affiliate
    if (affiliateId !== authenticatedAffiliateId) {
      return NextResponse.json(
        { error: 'Access denied. You can only create links for your own account.' },
        { status: 403 }
      )
    }

    // Verify affiliate exists and is active
    const affiliate = await affiliateService.getAffiliate(affiliateId)
    if (!affiliate) {
      return NextResponse.json(
        { error: 'Affiliate not found' },
        { status: 404 }
      )
    }

    if (affiliate.status !== 'active') {
      return NextResponse.json(
        { error: 'Affiliate is not active' },
        { status: 400 }
      )
    }

    // Generate the link
    const input: CreateAffiliateLinkInput = {
      affiliateId,
      affiliateCode,
      type: type || 'custom',
      targetUrl,
      utmCampaign: utmCampaign || 'affiliate',
      utmMedium,
      utmContent,
      utmTerm,
      expiresAt: expiresAt ? new Date(expiresAt) : undefined,
      metadata,
    }

    const link = affiliateLinkGenerator.generateLink(input)

    // Store the link
    links.set(link.id, link)
    shortCodeIndex.set(link.shortCode, link.id)

    // Index by affiliate
    if (!linksByAffiliate.has(affiliateId)) {
      linksByAffiliate.set(affiliateId, new Set())
    }
    linksByAffiliate.get(affiliateId)!.add(link.id)

    // Generate additional URL formats
    const response = {
      link,
      urls: {
        full: link.targetUrl,
        short: affiliateLinkGenerator.generateShortUrl(link.shortCode),
        shareable: affiliateLinkGenerator.generateShareableLink(affiliateId, affiliateCode, {
          url: link.targetUrl,
        }),
      },
    }

    return NextResponse.json(response, { status: 201 })
  } catch (error) {
    console.error('Create link error:', error)
    return NextResponse.json({ error: 'Failed to create link' }, { status: 500 })
  }
}

export async function PATCH(request: NextRequest) {
  try {
    const auth = authenticateRequest(request)
    if (!auth.success) return auth.response
    const { affiliateId } = auth

    const body = await request.json()
    const { linkId, status, expiresAt } = body

    if (!linkId) {
      return NextResponse.json(
        { error: 'linkId is required' },
        { status: 400 }
      )
    }

    // Verify link ownership
    const verification = verifyLinkOwnership(linkId, affiliateId)
    if (!verification.success) return verification.response
    const link = verification.link

    // Update fields
    if (status) {
      link.status = status
    }
    if (expiresAt !== undefined) {
      link.expiresAt = expiresAt ? new Date(expiresAt) : undefined
    }

    links.set(linkId, link)

    return NextResponse.json({ link })
  } catch (error) {
    console.error('Update link error:', error)
    return NextResponse.json({ error: 'Failed to update link' }, { status: 500 })
  }
}

export async function DELETE(request: NextRequest) {
  try {
    const auth = authenticateRequest(request)
    if (!auth.success) return auth.response
    const { affiliateId } = auth

    const { searchParams } = new URL(request.url)
    const linkId = searchParams.get('linkId')

    if (!linkId) {
      return NextResponse.json(
        { error: 'linkId is required' },
        { status: 400 }
      )
    }

    // Verify link ownership
    const verification = verifyLinkOwnership(linkId, affiliateId)
    if (!verification.success) return verification.response
    const link = verification.link

    // Soft delete - just mark as deleted
    link.status = 'deleted'
    links.set(linkId, link)

    return NextResponse.json({ success: true })
  } catch (error) {
    console.error('Delete link error:', error)
    return NextResponse.json({ error: 'Failed to delete link' }, { status: 500 })
  }
}