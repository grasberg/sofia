/**
 * GET /api/affiliate/stats - Get affiliate statistics
 * Returns aggregated data for affiliate performance
 */
import { NextRequest, NextResponse } from 'next/server'
import { verifyApiKey } from '@/lib/auth'
import { affiliateService } from '@/lib/AffiliateService'

// Import from track route (in production, use shared database)
interface ClickData {
  id: string
  source: string
  medium: string
  campaign: string
  landingPage: string
  timestamp: number
  converted: boolean
  affiliateId?: string
}

interface ConversionData {
  id: string
  clickId: string
  source: string
  value: number
  timestamp: number
  affiliateId?: string
}

// Shared state for demo (use database in production)
const clicks: Map<string, ClickData> = new Map()
const conversions: Map<string, ConversionData> = new Map()

export async function GET(request: NextRequest) {
  // Authenticate request
  const authResult = verifyApiKey(request)
  if (!authResult.authenticated) {
    return NextResponse.json(
      { error: authResult.error || 'Unauthorized' },
      { status: authResult.status || 401 }
    )
  }
  const affiliateId = authResult.affiliateId!

  const searchParams = request.nextUrl.searchParams
  const source = searchParams.get('source')
  const period = searchParams.get('period') || '30d' // 7d, 30d, 90d, all

  // If source is provided, verify it matches authenticated affiliate
  if (source && source !== affiliateId) {
    // Check if source is affiliate code
    try {
      const affiliate = await affiliateService.getAffiliateByCode(source)
      if (!affiliate || affiliate.id !== affiliateId) {
        return NextResponse.json(
          { error: 'Access denied. You can only view your own statistics.' },
          { status: 403 }
        )
      }
    } catch (error) {
      // If can't verify, deny access
      return NextResponse.json(
        { error: 'Access denied. Invalid source parameter.' },
        { status: 403 }
      )
    }
  }

  // Calculate period cutoff
  const now = Date.now()
  let cutoff = 0
  switch (period) {
    case '7d': cutoff = now - 7 * 24 * 60 * 60 * 1000; break
    case '30d': cutoff = now - 30 * 24 * 60 * 60 * 1000; break
    case '90d': cutoff = now - 90 * 24 * 60 * 60 * 1000; break
    default: cutoff = 0
  }

  // Filter by period and affiliate ID
  // In production, we would query database with affiliate ID filter
  const filteredClicks = Array.from(clicks.values()).filter(c => 
    c.timestamp >= cutoff && (c.affiliateId === affiliateId || c.source === affiliateId)
  )
  const filteredConversions = Array.from(conversions.values()).filter(c => 
    c.timestamp >= cutoff && (c.affiliateId === affiliateId || c.source === affiliateId)
  )

  // Calculate stats
  const totalClicks = filteredClicks.length
  const totalConversions = filteredConversions.length
  const totalValue = filteredConversions.reduce((sum, c) => sum + c.value, 0)
  const conversionRate = totalClicks > 0 ? (totalConversions / totalClicks) * 100 : 0
  const averageOrderValue = totalConversions > 0 ? totalValue / totalConversions : 0

  // Group by source (campaign)
  const bySource = filteredClicks.reduce((acc, click) => {
    const sourceKey = click.campaign || click.source || 'direct'
    if (!acc[sourceKey]) {
      acc[sourceKey] = { clicks: 0, conversions: 0, value: 0 }
    }
    acc[sourceKey].clicks++
    return acc
  }, {} as Record<string, { clicks: number; conversions: number; value: number }>)

  filteredConversions.forEach(conv => {
    const click = clicks.get(conv.clickId)
    if (click) {
      const sourceKey = click.campaign || click.source || 'direct'
      if (bySource[sourceKey]) {
        bySource[sourceKey].conversions++
        bySource[sourceKey].value += conv.value
      }
    }
  })

  // Top landing pages
  const byLandingPage = filteredClicks.reduce((acc, click) => {
    if (!acc[click.landingPage]) {
      acc[click.landingPage] = { views: 0 }
    }
    acc[click.landingPage].views++
    return acc
  }, {} as Record<string, { views: number }>)

  // Recent activity (last 10 clicks)
  const recentClicks = filteredClicks
    .sort((a, b) => b.timestamp - a.timestamp)
    .slice(0, 10)
    .map(click => ({
      id: click.id,
      source: click.source,
      medium: click.medium,
      campaign: click.campaign,
      landingPage: click.landingPage,
      timestamp: new Date(click.timestamp).toISOString(),
      converted: click.converted,
    }))

  return NextResponse.json({
    affiliateId,
    period,
    summary: {
      clicks: totalClicks,
      conversions: totalConversions,
      value: totalValue,
      conversionRate: conversionRate.toFixed(2) + '%',
      averageOrderValue: averageOrderValue.toFixed(2),
    },
    bySource: Object.entries(bySource).map(([source, stats]) => ({
      source,
      ...stats,
      conversionRate: stats.clicks > 0 ? ((stats.conversions / stats.clicks) * 100).toFixed(2) + '%' : '0%',
    })),
    topLandingPages: Object.entries(byLandingPage)
      .sort((a, b) => b[1].views - a[1].views)
      .slice(0, 5)
      .map(([page, data]) => ({ page, views: data.views })),
    recentActivity: recentClicks,
    generatedAt: new Date().toISOString(),
  })
}