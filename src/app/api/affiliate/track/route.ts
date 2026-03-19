/**
 * POST /api/affiliate/track - Record affiliate click
 * GET  /api/affiliate/track - Get current affiliate data
 */
import { NextRequest, NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import { affiliateService } from '@/lib/AffiliateService'

// In-memory store for demo (replace with DB in production)
const clickStore: Map<string, {
  id: string
  visitorId: string
  source: string
  medium: string
  campaign: string
  clickId?: string
  landingPage: string
  timestamp: number
  converted: boolean
  affiliateId?: string
}> = new Map()

const conversionStore: Map<string, {
  id: string
  clickId: string
  visitorId: string
  value: number
  timestamp: number
  affiliateId?: string
}> = new Map()

export async function POST(request: NextRequest) {
  try {
    const body = await request.json()
    const { visitorId, source, medium, campaign, clickId, landingPage } = body

    if (!visitorId || !source) {
      return NextResponse.json(
        { error: 'visitorId and source are required' },
        { status: 400 }
      )
    }

    // Validate source (affiliate code or affiliate ID)
    let affiliateId: string | null = null
    try {
      const affiliate = await affiliateService.getAffiliateByCode(source)
      if (affiliate) {
        affiliateId = affiliate.id
        // Optionally check if affiliate is active
        if (affiliate.status !== 'active') {
          console.warn(`Affiliate ${affiliate.id} is not active`)
        }
      } else {
        // Maybe source is affiliate ID
        const affiliateById = await affiliateService.getAffiliate(source)
        if (affiliateById) {
          affiliateId = affiliateById.id
          if (affiliateById.status !== 'active') {
            console.warn(`Affiliate ${affiliateById.id} is not active`)
          }
        } else {
          console.warn(`Source ${source} is not a valid affiliate code or ID`)
          // Still allow tracking but mark as unknown affiliate
        }
      }
    } catch (error) {
      console.error('Error validating affiliate:', error)
      // Continue tracking anyway
    }

    const clickRecord = {
      id: `clk_${Date.now()}_${Math.random().toString(36).slice(2, 9)}`,
      visitorId,
      source,
      medium: medium || 'affiliate',
      campaign: campaign || '',
      clickId,
      landingPage: landingPage || '/',
      timestamp: Date.now(),
      converted: false,
      affiliateId: affiliateId || undefined,
    }

    clickStore.set(clickRecord.id, clickRecord)

    // Set cookie for visitor tracking
    const cookieStore = await cookies()
    cookieStore.set('affiliate_click_id', clickRecord.id, {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'lax',
      maxAge: 30 * 24 * 60 * 60, // 30 days
    })
    cookieStore.set('affiliate_source', source, {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'lax',
      maxAge: 30 * 24 * 60 * 60,
    })

    return NextResponse.json({
      success: true,
      clickId: clickRecord.id,
      affiliateId,
      message: 'Click tracked successfully',
    })
  } catch (error) {
    console.error('Track error:', error)
    return NextResponse.json(
      { error: 'Failed to track click' },
      { status: 500 }
    )
  }
}

export async function GET() {
  try {
    const cookieStore = await cookies()
    const affiliateClickId = cookieStore.get('affiliate_click_id')?.value
    const affiliateSource = cookieStore.get('affiliate_source')?.value

    return NextResponse.json({
      clickId: affiliateClickId || null,
      source: affiliateSource || null,
      tracked: !!affiliateClickId,
    })
  } catch (error) {
    return NextResponse.json(
      { error: 'Failed to get tracking data' },
      { status: 500 }
    )
  }
}