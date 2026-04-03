<?php

return [
    /*
    |--------------------------------------------------------------------------
    | Affiliate Settings
    |--------------------------------------------------------------------------
    |
    | Configure your affiliate program settings here.
    |
    */

    // Default commission rate for new affiliate links
    'default_commission_rate' => env('AFFILIATE_DEFAULT_COMMISSION_RATE', 10.0),

    // Default commission type ('percentage' or 'fixed')
    'default_commission_type' => env('AFFILIATE_DEFAULT_COMMISSION_TYPE', 'percentage'),

    // Cookie lifetime in days
    'cookie_days' => env('AFFILIATE_COOKIE_DAYS', 30),

    // Maximum affiliate links per user (null = unlimited)
    'max_links_per_user' => env('AFFILIATE_MAX_LINKS_PER_USER', null),

    // Require approval for new affiliate accounts
    'require_approval' => env('AFFILIATE_REQUIRE_APPROVAL', false),

    // Minimum payout threshold
    'minimum_payout' => env('AFFILIATE_MINIMUM_PAYOUT', 100.0),

    // Default currency
    'currency' => env('AFFILIATE_CURRENCY', 'SEK'),

    // Conversion tracking
    'track_conversions' => [
        // Enable/disable various tracking features
        'device_detection' => true,
        'geo_tracking' => true,
        'utm_parameters' => true,
        'referrer_tracking' => true,
    ],

    // Fraud prevention
    'fraud_prevention' => [
        // Block multiple clicks from same IP in time window (seconds)
        'click_ip_window' => 60,
        // Block self-referrals
        'block_self_referrals' => true,
        // Require minimum time between click and conversion (seconds)
        'min_conversion_time' => 0,
    ],

    // Commission rules by conversion type
    'commission_rules' => [
        'sale' => [
            'type' => 'percentage',
            'rate' => 10.0,
        ],
        'lead' => [
            'type' => 'fixed',
            'amount' => 25.0,
        ],
        'signup' => [
            'type' => 'fixed',
            'amount' => 10.0,
        ],
    ],

    // Payout schedules
    'payout' => [
        // Payout frequency ('weekly', 'monthly', 'manual')
        'frequency' => env('AFFILIATE_PAYOUT_FREQUENCY', 'monthly'),
        // Payout day of month (for monthly)
        'payout_day' => 15,
        // Processing fee percentage
        'processing_fee_percent' => 0,
    ],

    // Email notifications
    'notifications' => [
        'new_conversion' => true,
        'payout_processed' => true,
        'commission_approved' => true,
        'link_expiring' => true,
    ],

    // API settings
    'api' => [
        'enabled' => true,
        'rate_limit' => 60, // requests per minute
        'require_auth' => true,
    ],

    // Cache settings
    'cache' => [
        'enabled' => true,
        'ttl' => 60, // seconds
    ],
];
