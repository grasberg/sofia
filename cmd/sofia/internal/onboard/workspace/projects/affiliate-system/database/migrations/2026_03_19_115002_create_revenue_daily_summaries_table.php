<?php

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
        Schema::create('revenue_daily_summaries', function (Blueprint $table) {
            $table->id();
            $table->date('date');
            $table->enum('channel', ['affiliate', 'consulting', 'product']);
            $table->decimal('total_amount', 12, 2)->default(0);
            $table->integer('transaction_count')->default(0);
            $table->timestamps();

            $table->unique(['date', 'channel']);
            $table->index('date');
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('revenue_daily_summaries');
    }
};