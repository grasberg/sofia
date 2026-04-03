<?php

namespace Database\Factories;

use App\Models\AffiliateConversion;
use Illuminate\Database\Eloquent\Factories\Factory;

class AffiliateConversionFactory extends Factory
{
    /**
     * The name of the factory's corresponding model.
     *
     * @var string
     */
    protected $model = AffiliateConversion::class;

    /**
     * Define the model's default state.
     *
     * @return array<string, mixed>
     */
    public function definition(): array
    {
        return [
            'affiliate_link_id' => \App\Models\AffiliateLink::factory(),
            'affiliate_click_id' => \App\Models\AffiliateClick::factory(),
            'user_id' => \App\Models\User::factory(),
            'order_id' => $this->faker->unique()->regexify('[A-Z]{3}-[0-9]{6}'),
            'order_value' => $this->faker->randomFloat(2, 10, 1000),
            'commission_amount' => $this->faker->randomFloat(2, 1, 200),
            'conversion_type' => $this->faker->randomElement(['sale', 'lead', 'signup', 'download']),
            'conversion_data' => [
                'product_name' => $this->faker->word(),
                'customer_email' => $this->faker->email(),
            ],
            'status' => 'pending',
            'converted_at' => $this->faker->dateTimeBetween('-30 days', 'now'),
            'paid_at' => null,
        ];
    }

    /**
     * Indicate that the conversion is pending.
     */
    public function pending(): static
    {
        return $this->state(fn (array $attributes) => [
            'status' => 'pending',
            'paid_at' => null,
        ]);
    }

    /**
     * Indicate that the conversion is approved.
     */
    public function approved(): static
    {
        return $this->state(fn (array $attributes) => [
            'status' => 'approved',
            'paid_at' => null,
        ]);
    }

    /**
     * Indicate that the conversion is rejected.
     */
    public function rejected(): static
    {
        return $this->state(fn (array $attributes) => [
            'status' => 'rejected',
            'paid_at' => null,
        ]);
    }

    /**
     * Indicate that the conversion is paid.
     */
    public function paid(): static
    {
        return $this->state(fn (array $attributes) => [
            'status' => 'paid',
            'paid_at' => $this->faker->dateTimeBetween('-7 days', 'now'),
        ]);
    }

    /**
     * Indicate that the conversion is for a specific order value.
     */
    public function orderValue(float $value): static
    {
        return $this->state(fn (array $attributes) => [
            'order_value' => $value,
        ]);
    }

    /**
     * Indicate that the conversion is of a specific type.
     */
    public function conversionType(string $type): static
    {
        return $this->state(fn (array $attributes) => [
            'conversion_type' => $type,
        ]);
    }

    /**
     * Indicate that the conversion occurred at a specific time.
     */
    public function convertedAt(\DateTimeInterface $date): static
    {
        return $this->state(fn (array $attributes) => [
            'converted_at' => $date,
        ]);
    }
}