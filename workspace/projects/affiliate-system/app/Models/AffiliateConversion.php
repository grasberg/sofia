<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

class AffiliateConversion extends Model
{
    use HasFactory;

    protected $fillable = [
        'affiliate_link_id',
        'affiliate_click_id',
        'user_id',
        'order_id',
        'order_value',
        'commission_amount',
        'status',
        'currency',
        'conversion_type',
        'conversion_data',
        'notes',
        'converted_at',
        'approved_at',
    ];

    protected $casts = [
        'order_value' => 'decimal:2',
        'commission_amount' => 'decimal:2',
        'conversion_data' => 'array',
        'converted_at' => 'datetime',
        'approved_at' => 'datetime',
    ];

    public function affiliateLink(): BelongsTo
    {
        return $this->belongsTo(AffiliateLink::class);
    }

    public function affiliateClick(): BelongsTo
    {
        return $this->belongsTo(AffiliateClick::class);
    }

    public function user(): BelongsTo
    {
        return $this->belongsTo(User::class);
    }

    // Scopes
    public function scopePending($query)
    {
        return $query->where('status', 'pending');
    }

    public function scopeApproved($query)
    {
        return $query->where('status', 'approved');
    }

    public function scopePaid($query)
    {
        return $query->where('status', 'paid');
    }
}
