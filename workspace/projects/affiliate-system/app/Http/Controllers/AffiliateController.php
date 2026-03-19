<?php

namespace App\Http\Controllers;

use App\Models\AffiliateLink;
use App\Models\AffiliateClick;
use App\Models\AffiliateConversion;
use Illuminate\Http\Request;
use Illuminate\Http\JsonResponse;
use Illuminate\Support\Facades\Auth;
use Illuminate\Support\Facades\Cache;
use Illuminate\Support\Facades\Log;

class AffiliateController extends Controller
{
    /**
     * Track a click on an affiliate link
     */
    public function track(string $code, Request $request): JsonResponse
    {
        $link = AffiliateLink::where('code', $code)->first();

        if (!$link || !$link->isValid()) {
            return redirect()->away($link->target_url ?? '/');
        }

        // Extract UTM data
        $utmData = [
            'utm_source' => $request->get('utm_source'),
            'utm_medium' => $request->get('utm_medium'),
            'utm_campaign' => $request->get('utm_campaign'),
            'utm_term' => $request->get('utm_term'),
            'utm_content' => $request->get('utm_content'),
        ];

        // Detect device type
        $userAgent = $request->userAgent();
        $deviceType = $this->detectDeviceType($userAgent);

        // Detect browser
        $browser = $this->detectBrowser($userAgent);

        // Detect OS
        $os = $this->detectOS($userAgent);

        // Record click
        $click = $link->recordClick([
            'user_id' => Auth::id(),
            'ip_address' => $request->ip(),
            'user_agent' => $userAgent,
            'referer' => $request->header('referer'),
            'country_code' => $request->get('country', 'SE'),
            'device_type' => $deviceType,
            'browser' => $browser,
            'os' => $os,
            'utm_data' => array_filter($utmData),
            'session_id' => $request->session()->getId(),
        ]);

        // Update commission summary
        $this->updateCommissionSummary($link->user_id);

        // Redirect to target URL with click ID for conversion tracking
        $targetUrl = $link->target_url;
        $separator = parse_url($targetUrl, PHP_URL_QUERY) ? '&' : '?';
        $targetUrl .= "{$separator}affiliate_click={$click->id}";

        return redirect()->away($targetUrl);
    }

    /**
     * Create a new affiliate link
     */
    public function createLink(Request $request): JsonResponse
    {
        $validated = $request->validate([
            'name' => 'required|string|max:255',
            'target_url' => 'required|url',
            'commission_rate' => 'nullable|numeric|min:0|max:100',
            'commission_type' => 'nullable|in:percentage,fixed',
            'commission_fixed' => 'nullable|numeric|min:0',
            'max_conversions' => 'nullable|integer|min:1',
            'starts_at' => 'nullable|date',
            'expires_at' => 'nullable|date|after:starts_at',
        ]);

        $link = AffiliateLink::create([
            'user_id' => Auth::id(),
            'code' => \Illuminate\Support\Str::random(12),
            ...$validated,
        ]);

        return response()->json([
            'success' => true,
            'data' => $link,
            'tracking_url' => $link->tracking_url,
        ], 201);
    }

    /**
     * Get affiliate link(s)
     */
    public function getLink(Request $request): JsonResponse
    {
        if ($code = $request->get('code')) {
            $link = AffiliateLink::where('code', $code)
                ->where('user_id', Auth::id())
                ->first();

            if (!$link) {
                return response()->json(['success' => false, 'error' => 'Link not found'], 404);
            }

            return response()->json([
                'success' => true,
                'data' => $link,
                'stats' => $link->getStats(),
            ]);
        }

        $links = AffiliateLink::where('user_id', Auth::id())
            ->withCount(['clicks', 'conversions'])
            ->orderBy('created_at', 'desc')
            ->paginate(20);

        return response()->json([
            'success' => true,
            'data' => $links,
        ]);
    }

    /**
     * Record a conversion
     */
    public function recordConversion(Request $request): JsonResponse
    {
        $validated = $request->validate([
            'click_id' => 'nullable|exists:affiliate_clicks,id',
            'order_id' => 'nullable|string|max:255',
            'order_value' => 'required|numeric|min:0',
            'conversion_type' => 'required|string|max:50',
            'conversion_data' => 'nullable|array',
        ]);

        $click = AffiliateClick::find($validated['click_id']);
        
        if ($click) {
            $link = $click->affiliateLink;
            $userId = $click->user_id;
        } else {
            // Try to find link by code from request
            $linkCode = $request->get('affiliate_code');
            $link = AffiliateLink::where('code', $linkCode)->first();
            $userId = $link?->user_id;
        }

        if (!$link) {
            return response()->json(['success' => false, 'error' => 'Invalid affiliate link'], 400);
        }

        $commission = $link->calculateCommission($validated['order_value']);

        $conversion = AffiliateConversion::create([
            'affiliate_link_id' => $link->id,
            'affiliate_click_id' => $click?->id,
            'user_id' => $userId,
            'order_id' => $validated['order_id'] ?? null,
            'order_value' => $validated['order_value'],
            'commission_amount' => $commission,
            'conversion_type' => $validated['conversion_type'],
            'conversion_data' => $validated['conversion_data'] ?? null,
            'converted_at' => now(),
        ]);

        // Update summary
        $this->updateCommissionSummary($link->user_id);

        Log::info('Affiliate conversion recorded', [
            'conversion_id' => $conversion->id,
            'affiliate_link_id' => $link->id,
            'commission' => $commission,
        ]);

        return response()->json([
            'success' => true,
            'data' => $conversion,
        ], 201);
    }

    /**
     * Get affiliate statistics
     */
    public function getStats(Request $request): JsonResponse
    {
        $userId = Auth::id();
        $cacheKey = "affiliate_stats_{$userId}";

        $stats = Cache::remember($cacheKey, 60, function () use ($userId) {
            $links = AffiliateLink::where('user_id', $userId)->get();
            $linkIds = $links->pluck('id');

            return [
                'total_links' => $links->count(),
                'active_links' => $links->where('status', 'active')->count(),
                'total_clicks' => AffiliateClick::whereIn('affiliate_link_id', $linkIds)->count(),
                'total_conversions' => AffiliateConversion::whereIn('affiliate_link_id', $linkIds)->count(),
                'pending_conversions' => AffiliateConversion::whereIn('affiliate_link_id', $linkIds)
                    ->where('status', 'pending')->count(),
                'approved_conversions' => AffiliateConversion::whereIn('affiliate_link_id', $linkIds)
                    ->where('status', 'approved')->count(),
                'rejected_conversions' => AffiliateConversion::whereIn('affiliate_link_id', $linkIds)
                    ->where('status', 'rejected')->count(),
                'total_earnings' => AffiliateConversion::whereIn('affiliate_link_id', $linkIds)
                    ->whereIn('status', ['approved', 'paid'])->sum('commission_amount'),
                'pending_earnings' => AffiliateConversion::whereIn('affiliate_link_id', $linkIds)
                    ->where('status', 'pending')->sum('commission_amount'),
                'paid_earnings' => AffiliateConversion::whereIn('affiliate_link_id', $linkIds)
                    ->where('status', 'paid')->sum('commission_amount'),
                'conversion_rate' => AffiliateClick::whereIn('affiliate_link_id', $linkIds)->count() > 0
                    ? round(
                        (AffiliateConversion::whereIn('affiliate_link_id', $linkIds)->count() /
                         AffiliateClick::whereIn('affiliate_link_id', $linkIds)->count()) * 100,
                        2
                      )
                    : 0,
            ];
        });

        return response()->json([
            'success' => true,
            'data' => $stats,
        ]);
    }

    /**
     * Update commission summary for a user
     */
    private function updateCommissionSummary(int $userId): void
    {
        $linkIds = AffiliateLink::where('user_id', $userId)->pluck('id');

        $summary = [
            'total_clicks' => AffiliateClick::whereIn('affiliate_link_id', $linkIds)->count(),
            'total_conversions' => AffiliateConversion::whereIn('affiliate_link_id', $linkIds)->count(),
            'pending_commissions' => AffiliateConversion::whereIn('affiliate_link_id', $linkIds)
                ->where('status', 'pending')->sum('commission_amount'),
            'approved_commissions' => AffiliateConversion::whereIn('affiliate_link_id', $linkIds)
                ->where('status', 'approved')->sum('commission_amount'),
            'paid_commissions' => AffiliateConversion::whereIn('affiliate_link_id', $linkIds)
                ->where('status', 'paid')->sum('commission_amount'),
            'total_earned' => AffiliateConversion::whereIn('affiliate_link_id', $linkIds)
                ->whereIn('status', ['approved', 'paid'])->sum('commission_amount'),
            'last_click_at' => AffiliateClick::whereIn('affiliate_link_id', $linkIds)->max('clicked_at'),
            'last_conversion_at' => AffiliateConversion::whereIn('affiliate_link_id', $linkIds)->max('converted_at'),
        ];

        \App\Models\AffiliateCommissionSummary::updateOrCreate(
            ['user_id' => $userId],
            $summary
        );
    }

    /**
     * Detect device type from user agent
     */
    private function detectDeviceType(?string $userAgent): string
    {
        if (!$userAgent) return 'unknown';

        if (preg_match('/mobile|android|iphone|ipad|ipod/i', $userAgent)) {
            if (preg_match('/tablet|ipad/i', $userAgent)) {
                return 'tablet';
            }
            return 'mobile';
        }

        return 'desktop';
    }

    /**
     * Detect browser from user agent
     */
    private function detectBrowser(?string $userAgent): string
    {
        if (!$userAgent) return 'unknown';

        $browsers = [
            'Edge' => '/edg/i',
            'Chrome' => '/chrome/i',
            'Firefox' => '/firefox/i',
            'Safari' => '/safari/i',
            'Opera' => '/opera|opr/i',
            'IE' => '/msie|trident/i',
        ];

        foreach ($browsers as $browser => $pattern) {
            if (preg_match($pattern, $userAgent)) {
                return $browser;
            }
        }

        return 'other';
    }

    /**
     * Detect OS from user agent
     */
    private function detectOS(?string $userAgent): string
    {
        if (!$userAgent) return 'unknown';

        $operatingSystems = [
            'iOS' => '/iphone|ipad|ipod/i',
            'Android' => '/android/i',
            'Windows' => '/win/i',
            'Mac' => '/mac/i',
            'Linux' => '/linux/i',
            'Ubuntu' => '/ubuntu/i',
        ];

        foreach ($operatingSystems as $os => $pattern) {
            if (preg_match($pattern, $userAgent)) {
                return $os;
            }
        }

        return 'other';
    }
}
