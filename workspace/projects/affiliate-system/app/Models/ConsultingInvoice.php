<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;
use Illuminate\Database\Eloquent\SoftDeletes;

class ConsultingInvoice extends Model
{
    use HasFactory, SoftDeletes;

    protected $fillable = [
        'user_id',
        'client_name',
        'client_email',
        'project_name',
        'hourly_rate',
        'hours',
        'total_amount',
        'currency',
        'status',
        'service_date',
        'paid_at',
        'notes',
        'metadata',
    ];

    protected $casts = [
        'hourly_rate' => 'decimal:2',
        'hours' => 'decimal:2',
        'total_amount' => 'decimal:2',
        'service_date' => 'date',
        'paid_at' => 'datetime',
        'metadata' => 'array',
    ];

    public function user(): BelongsTo
    {
        return $this->belongsTo(User::class);
    }

    // Scopes
    public function scopePaid($query)
    {
        return $query->where('status', 'paid');
    }

    public function scopePending($query)
    {
        return $query->whereIn('status', ['draft', 'sent']);
    }

    public function scopeOverdue($query)
    {
        return $query->where('status', 'overdue');
    }

    public function scopeForDateRange($query, $start, $end)
    {
        return $query->whereBetween('service_date', [$start, $end]);
    }
}