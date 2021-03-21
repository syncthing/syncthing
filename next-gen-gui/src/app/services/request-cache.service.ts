import { Injectable } from '@angular/core';
import { HttpResponse, HttpRequest } from '@angular/common/http';

export interface RequestCacheEntry {
  url: string;
  response: HttpResponse<any>;
  lastRead: number;
}

const maxAge = 30000; // milliseconds

@Injectable({
  providedIn: 'root'
})
export class RequestCacheService {
  private cache: Map<string, RequestCacheEntry> = new Map();

  constructor() { }

  get(req: HttpRequest<any>): HttpResponse<any> | undefined {
    const url = req.urlWithParams;
    const cached = this.cache.get(url);

    if (!cached) {
      return undefined;
    }

    const isExpired = cached.lastRead < (Date.now() - maxAge);
    return isExpired ? undefined : cached.response;
  }

  put(req: HttpRequest<any>, response: HttpResponse<any>): void {
    const url = req.urlWithParams;

    const entry = { url, response, lastRead: Date.now() };
    this.cache.set(url, entry);

    // Remove expired cache entries
    const expired = Date.now() - maxAge;
    this.cache.forEach(entry => {
      if (entry.lastRead < expired) {
        this.cache.delete(entry.url);
      }
    });
  }

  clearAll(): void {
    this.cache = new Map();
  }
}
