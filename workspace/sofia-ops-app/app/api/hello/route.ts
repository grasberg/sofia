import { NextRequest, NextResponse } from 'next/server';
import logger from '../../lib/logger';

export async function GET(request: NextRequest) {
  const searchParams = request.nextUrl.searchParams;
  const name = searchParams.get('name') || 'World';

  logger.info({ endpoint: '/api/hello', name }, 'API endpoint called');

  // Simulate some processing
  await new Promise(resolve => setTimeout(resolve, 100));

  logger.debug({ endpoint: '/api/hello' }, 'Processing complete');

  return NextResponse.json({
    message: `Hello ${name}!`,
    timestamp: new Date().toISOString(),
  });
}

export async function POST(request: NextRequest) {
  try {
    const body = await request.json();
    logger.info({ endpoint: '/api/hello', body }, 'POST request received');

    // Validate input
    if (!body.name) {
      logger.warn({ endpoint: '/api/hello' }, 'Missing name in request');
      return NextResponse.json(
        { error: 'Name is required' },
        { status: 400 }
      );
    }

    return NextResponse.json({
      message: `Hello ${body.name}!`,
      received: body,
    });
  } catch (error) {
    logger.error({ endpoint: '/api/hello', error }, 'Failed to process POST request');
    return NextResponse.json(
      { error: 'Invalid JSON' },
      { status: 400 }
    );
  }
}