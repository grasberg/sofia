<?php

namespace Database\Factories;

use App\Models\AffiliateLink;
use Illuminate\Database\Eloquent\Factories\Factory;
use Illuminate\Support\Str;

class AffiliateLinkFactory extends Factory
{
    /**
     * The name of the factory's corresponding model.
     *
     * @var string
     */
    protected $model = AffiliateLink::class;

    /**
     * Define the model's default state.
     *
     * @return array<string, mixed>
     */
    public function definition(): array
    {
        return [
            'user_id' => \App\Models\User::factory(),
            'code' => Str::random(12),
            'name' => $this->faker->words(3, true),
            'target_url' => $this->faker->url(),
            'commission_rate' => $this->faker->randomFloat(2, 1, 50),
            'commission_type' => $this->faker->randomElement(['percentage', 'fixed']),
            'commission_fixed' => $this->faker->randomFloat(2, 5, 100),
            'status' => 'active',
            'max_conversions' => null,
            'starts_at' => null,
            'expires_at' => null,
            'metadata' => null,
        ];
    }

    /**
     * Indicate that the link is active.
     */
    public function active(): static
    {
        return $this->state(fn (array $attributes) => [
            'status' => 'active',
        ]);
    }

    /**
     * Indicate that the link is inactive.
     */
    public function inactive(): static
    {
        return $this->state(fn (array $attributes) => [
            'status' => 'inactive',
        ]);
    }

    /**
     * Indicate that the link has percentage commission.
     */
    public function percentageCommission(float $rate = null): static
    {
        return $this->state(fn (array $attributes) => [
            'commission_type' => 'percentage',
            'commission_rate' => $rate ?? $this->faker->randomFloat(2, 1, 30),
            'commission_fixed' => null,
        ]);
    }

    /**
     * Indicate that the link has fixed commission.
     */
    public function fixedCommission(float $amount = null): static
    {
        return $this->state(fn (array $attributes) => [
            'commission_type' => 'fixed',
            'commission_fixed' => $amount ?? $this->faker->randomFloat(2, 5, 100),
            'commission_rate' => null,
        ]);
    }

    /**
     * Indicate that the link expires in the future.
     */
    public function expiresInFuture(int $days = 7): static
    {
        return $this->state(fn (array $attributes) => [
            'expires_at' => now()->addDays($days),
        ]);
    }

    /**
     * Indicate that the link expired in the past.
     */
    public function expired(): static
    {
        return $this->state(fn (array $attributes) => [
            'expires_at' => now()->subDays(7),
        ]);
    }

    /**
     * Indicate that the link starts in the future.
     */
    public function startsInFuture(int $days = 7): static
    {
        return $this->state(fn (array $attributes) => [
            'starts_at' => now()->addDays($days),
        ]);
    }

    /**
     * Indicate that the link has a maximum conversion limit.
     */
    public function withMaxConversions(int $max): static
    {
        return $this->state(fn (array $attributes) => [
            'max_conversions' => $max,
        ]);
    }
}