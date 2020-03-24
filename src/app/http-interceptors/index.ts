/* "Barrel" of Http Interceptors */
import { HTTP_INTERCEPTORS } from '@angular/common/http';

import { CSRFInterceptor } from './csrf.interceptor';
import { CachingInterceptor } from './caching.interceptor';

/** Http interceptor providers in outside-in order */
export const httpInterceptorProviders = [
    { provide: HTTP_INTERCEPTORS, useClass: CachingInterceptor, multi: true },
    // CSRFInterceptor needs to be last 
    { provide: HTTP_INTERCEPTORS, useClass: CSRFInterceptor, multi: true },
];