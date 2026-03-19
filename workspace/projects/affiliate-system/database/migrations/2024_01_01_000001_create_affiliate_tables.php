<?php
/**
 * Affiliate Database Schema
 * 
 * Migrations for affiliate tracking system
 */

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    /**
     * Run the migrations.
     */
    public function up(): void
    {
        // Affiliate Links Table
        Schema::create('affiliate_links', function (Blueprint $table) {
            $table->id();
            $table->foreignId('user_id')->constrained()->onDelete('cascade');
            $table->string('code', 32)->unique();
            $table->string('name');
            $table->string('target_url');
            $table->decimal('commission_rate', 5, 2)->default(10.00); // Percentage
            $table->enum('commission_type', ['percentage', 'fixed'])->default('percentage');
            $table->decimal('commission_fixed', 10, 2)->nullable();
            $table->enum('status', ['active', 'inactive', 'pending'])->default('active');
            $table->unsignedInteger('max_conversions')->nullable(); // null = unlimited
            $table->timestamp('starts_at')->nullable();
            $table->timestamp('expires_at')->nullable();
            $table->json('metadata')->nullable(); // For custom tracking data
            $table->timestamps();
            $table->softDeletes();

            $table->index(['user_id', 'status']);
            $table->index('code');
        });

        // Affiliate Clicks Table
        Schema::create('affiliate_clicks', function (Blueprint $table) {
            $table->id();
            $table->foreignId('affiliate_link_id')->constrained()->onDelete('cascade');
            $table->foreignId('user_id')->nullable()->constrained()->onDelete('set null');
            $table->string('ip_address', 45);
            $table->string('user_agent')->nullable();
            $table->string('referer')->nullable();
            $table->string('country_code', 2)->nullable();
            $table->string('device_type', 20)->nullable(); // mobile, tablet, desktop
            $table->string('browser', 50)->nullable();
            $table->string('os', 50)->nullable();
            $table->json('utm_data')->nullable(); // UTM parameters
            $table->string('session_id')->nullable();
            $table->timestamp('clicked_at');
            $table->timestamps();

            $table->index(['affiliate_link_id', 'clicked_at']);
            $table->index('ip_address');
            $table->index('session_id');
        });

        // Affiliate Conversions Table
        Schema::create('affiliate_conversions', function (Blueprint $table) {
            $table->id();
            $table->foreignId('affiliate_link_id')->constrained()->onDelete('cascade');
            $table->foreignId('affiliate_click_id')->nullable()->constrained()->onDelete('set null');
            $table->foreignId('user_id')->nullable()->constrained()->onDelete('set null');
            $table->string('order_id')->nullable();
            $table->decimal('order_value', 12, 2)->nullable();
            $table->decimal('commission_amount', 10, 2);
            $table->enum('status', ['pending', 'approved', 'rejected', 'paid'])->default('pending');
            $table->string('currency', 3)->default('SEK');
            $table->string('conversion_type', 50); // sale, lead, signup, etc.
            $table->json('conversion_data')->nullable(); // Custom conversion metadata
            $table->text('notes')->nullable();
            $table->timestamp('converted_at');
            $table->timestamp('approved_at')->nullable();
            $table->timestamps();

            $table->index(['affiliate_link_id', 'status']);
            $table->index('converted_at');
            $table->index('user_id');
        });

        // Affiliate Commissions Summary (for performance)
        Schema::create('affiliate_commission_summaries', function (Blueprint $table) {
            $table->id();
            $table->foreignId('user_id')->constrained()->onDelete('cascade');
            $table->decimal('total_clicks', 12, 0)->default(0);
            $table->decimal('total_conversions', 12, 0)->default(0);
            $table->decimal('pending_commissions', 12, 2)->default(0);
            $table->decimal('approved_commissions', 12, 2)->default(0);
            $table->decimal('paid_commissions', 12, 2)->default(0);
            $table->decimal('total_earned', 12, 2)->default(0);
            $table->timestamp('last_click_at')->nullable();
            $table->timestamp('last_conversion_at')->nullable();
            $table->timestamps();

            $table->unique('user_id');
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('affiliate_commission_summaries');
        Schema::dropIfExists('affiliate_conversions');
        Schema::dropIfExists('affiliate_clicks');
        Schema::dropIfExists('affiliate_links');
    }
};
