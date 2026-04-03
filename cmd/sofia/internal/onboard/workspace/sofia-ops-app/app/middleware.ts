import { NextRequest, NextResponse } from 'next/server';
import logger from '../lib/logger';

export function middleware(request: NextRequest) {
  const start = Date.now();
  const { method, url, headers } = request;
  const ip = request.ip || headers.get('x-forwarded-for') || headers.get('x-real-ip') || 'unknown';
  const userAgent = headers.get('user-agent') || 'unknown';

  // Log request
  logger.info(
    {
      method,
      url: url,
      ip,
      userAgent,
    },
    'Incoming request'
  );

  // Continue processing
  const response = NextResponse.next();

  // Log response
  const duration = Date.now() - start;
  logger.info(
    {
      method,
      url: url,
      status: response.status,
      duration,
    },
    'Request completed'
  );

  return response;
}

// Configure which paths middleware runs on
export const config = {
  matcher: [
    /*
     * Match all request paths except for the ones starting with:
     * - _next/static (static files)
     * - _next/image (image optimization files)
     * - favicon.ico (favicon file)
     * - public folder
     */
    '/((?!_next/static|_next/image|favicon.ico|public/).*)',
  ],
};