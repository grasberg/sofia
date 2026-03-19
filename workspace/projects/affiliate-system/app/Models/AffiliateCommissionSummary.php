<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

class AffiliateCommissionSummary extends Model
{
    use HasFactory;

    protected $table = 'affiliate_commission_summaries';

    protected $fillable = [
        'user_id',
        'total_clicks',
        'total_conversions',
        'pending_commissions',
        'approved_commissions',
        'paid_commissions',
        'total_earned',
        'last_click_at',
        'last_conversion_at',
    ];

    protected $casts = [
        'total_clicks' => 'integer',
        'total_conversions' => 'integer',
        'pending_commissions' => 'decimal:2',
        'approved_commissions' => 'decimal:2',
        'paid_commissions' => 'decimal:2',
        'total_earned' => 'decimal:2',
        'last_click_at' => 'datetime',
        'last_conversion_at' => 'datetime',
    ];

    public function user(): BelongsTo
    {
        return $this->belongsTo(User::class);
    }
}
