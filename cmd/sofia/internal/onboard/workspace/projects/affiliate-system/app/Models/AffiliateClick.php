<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

class AffiliateClick extends Model
{
    use HasFactory;

    protected $fillable = [
        'affiliate_link_id',
        'user_id',
        'ip_address',
        'user_agent',
        'referer',
        'country_code',
        'device_type',
        'browser',
        'os',
        'utm_data',
        'session_id',
        'clicked_at',
    ];

    protected $casts = [
        'utm_data' => 'array',
        'clicked_at' => 'datetime',
    ];

    public function affiliateLink(): BelongsTo
    {
        return $this->belongsTo(AffiliateLink::class);
    }

    public function user(): BelongsTo
    {
        return $this->belongsTo(User::class);
    }
}
