<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;
use Illuminate\Database\Eloquent\Relations\HasMany;
use Illuminate\Database\Eloquent\SoftDeletes;
use Illuminate\Support\Str;

class AffiliateLink extends Model
{
    use HasFactory, SoftDeletes;

    protected $fillable = [
        'user_id',
        'code',
        'name',
        'target_url',
        'commission_rate',
        'commission_type',
        'commission_fixed',
        'status',
        'max_conversions',
        'starts_at',
        'expires_at',
        'metadata',
    ];

    protected $casts = [
        'commission_rate' => 'decimal:2',
        'commission_fixed' => 'decimal:2',
        'max_conversions' => 'integer',
        'starts_at' => 'datetime',
        'expires_at' => 'datetime',
        'metadata' => 'array',
    ];

    protected $appends = ['tracking_url'];

    /**
     * Boot the model
     */
    protected static function boot(): void
    {
        parent::boot();

        static::creating(function ($link) {
            if (empty($link->code)) {
                $link->code = Str::random(12);
            }
        });
    }

    /**
     * Get the affiliate's tracking URL
     */
    public function getTrackingUrlAttribute(): string
    {
        return url("/api/affiliate/track/{$this->code}");
    }

    /**
     * Get the full tracking URL with optional parameters
     */
    public function getTrackingUrlWithParams(array $params = []): string
    {
        $url = $this->tracking_url;
        
        if (!empty($params)) {
            $url .= '?' . http_build_query($params);
        }
        
        return $url;
    }

    /**
     * Check if the link is valid and active
     */
    public function isValid(): bool
    {
        if ($this->status !== 'active') {
            return false;
        }

        if ($this->starts_at && $this->starts_at->isFuture()) {
            return false;
        }

        if ($this->expires_at && $this->expires_at->isPast()) {
            return false;
        }

        if ($this->max_conversions !== null) {
            $conversionCount = $this->conversions()->count();
            if ($conversionCount >= $this->max_conversions) {
                return false;
            }
        }

        return true;
    }

    /**
     * Calculate commission for a given order value
     */
    public function calculateCommission(float $orderValue): float
    {
        if ($this->commission_type === 'fixed') {
            return (float) $this->commission_fixed;
        }

        return ($orderValue * $this->commission_rate) / 100;
    }

    /**
     * Record a click
     */
    public function recordClick(array $clickData): AffiliateClick
    {
        return $this->clicks()->create([
            ...$clickData,
            'clicked_at' => now(),
        ]);
    }

    /**
     * Record a conversion
     */
    public function recordConversion(array $conversionData): AffiliateConversion
    {
        $commission = $this->calculateCommission($conversionData['order_value'] ?? 0);

        return $this->conversions()->create([
            ...$conversionData,
            'commission_amount' => $commission,
            'converted_at' => now(),
        ]);
    }

    /**
     * Get statistics for this affiliate link
     */
    public function getStats(): array
    {
        return [
            'total_clicks' => $this->clicks()->count(),
            'unique_clicks' => $this->clicks()->distinct('ip_address')->count(),
            'total_conversions' => $this->conversions()->count(),
            'pending_conversions' => $this->conversions()->where('status', 'pending')->count(),
            'approved_conversions' => $this->conversions()->where('status', 'approved')->count(),
            'rejected_conversions' => $this->conversions()->where('status', 'rejected')->count(),
            'total_commission' => $this->conversions()->whereIn('status', ['approved', 'paid'])->sum('commission_amount'),
            'pending_commission' => $this->conversions()->where('status', 'pending')->sum('commission_amount'),
            'conversion_rate' => $this->clicks()->count() > 0 
                ? round(($this->conversions()->count() / $this->clicks()->count()) * 100, 2)
                : 0,
        ];
    }

    // Relationships
    public function user(): BelongsTo
    {
        return $this->belongsTo(User::class);
    }

    public function clicks(): HasMany
    {
        return $this->hasMany(AffiliateClick::class);
    }

    public function conversions(): HasMany
    {
        return $this->hasMany(AffiliateConversion::class);
    }

    // Scopes
    public function scopeActive($query)
    {
        return $query->where('status', 'active');
    }

    public function scopeExpired($query)
    {
        return $query->whereNotNull('expires_at')
                     ->where('expires_at', '<', now());
    }

    public function scopeValid($query)
    {
        return $query->active()
                     ->where(function ($q) {
                         $q->whereNull('starts_at')
                           ->orWhere('starts_at', '<=', now());
                     })
                     ->where(function ($q) {
                         $q->whereNull('expires_at')
                           ->orWhere('expires_at', '>', now());
                     });
    }
}
