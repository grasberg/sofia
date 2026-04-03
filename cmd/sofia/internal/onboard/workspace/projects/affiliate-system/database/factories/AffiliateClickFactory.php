<?php

namespace Database\Factories;

use App\Models\AffiliateClick;
use Illuminate\Database\Eloquent\Factories\Factory;

class AffiliateClickFactory extends Factory
{
    /**
     * The name of the factory's corresponding model.
     *
     * @var string
     */
    protected $model = AffiliateClick::class;

    /**
     * Define the model's default state.
     *
     * @return array<string, mixed>
     */
    public function definition(): array
    {
        return [
            'affiliate_link_id' => \App\Models\AffiliateLink::factory(),
            'user_id' => \App\Models\User::factory(),
            'ip_address' => $this->faker->ipv4(),
            'user_agent' => $this->faker->userAgent(),
            'referer' => $this->faker->url(),
            'country_code' => $this->faker->countryCode(),
            'device_type' => $this->faker->randomElement(['desktop', 'mobile', 'tablet']),
            'browser' => $this->faker->randomElement(['Chrome', 'Firefox', 'Safari', 'Edge']),
            'os' => $this->faker->randomElement(['Windows', 'macOS', 'Linux', 'iOS', 'Android']),
            'utm_data' => [
                'utm_source' => $this->faker->randomElement(['google', 'facebook', 'twitter', 'email']),
                'utm_medium' => $this->faker->randomElement(['cpc', 'social', 'email']),
                'utm_campaign' => $this->faker->word(),
            ],
            'session_id' => $this->faker->uuid(),
            'clicked_at' => $this->faker->dateTimeBetween('-30 days', 'now'),
        ];
    }

    /**
     * Indicate that the click is from a mobile device.
     */
    public function mobile(): static
    {
        return $this->state(fn (array $attributes) => [
            'device_type' => 'mobile',
            'user_agent' => 'Mozilla/5.0 (iPhone; CPU iPhone OS 14_0 like Mac OS X) AppleWebKit/605.1.15',
        ]);
    }

    /**
     * Indicate that the click is from a desktop device.
     */
    public function desktop(): static
    {
        return $this->state(fn (array $attributes) => [
            'device_type' => 'desktop',
            'user_agent' => 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36',
        ]);
    }

    /**
     * Indicate that the click is from a specific country.
     */
    public function country(string $countryCode): static
    {
        return $this->state(fn (array $attributes) => [
            'country_code' => $countryCode,
        ]);
    }

    /**
     * Indicate that the click has UTM parameters.
     */
    public function withUtm(array $utmData): static
    {
        return $this->state(fn (array $attributes) => [
            'utm_data' => array_merge($attributes['utm_data'] ?? [], $utmData),
        ]);
    }

    /**
     * Indicate that the click occurred at a specific time.
     */
    public function clickedAt(\DateTimeInterface $date): static
    {
        return $this->state(fn (array $attributes) => [
            'clicked_at' => $date,
        ]);
    }
}