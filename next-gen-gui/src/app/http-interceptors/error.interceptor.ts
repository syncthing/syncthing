import { Injectable } from '@angular/core';
import {
  HttpRequest,
  HttpHandler,
  HttpEvent,
  HttpInterceptor,
  HttpErrorResponse
} from '@angular/common/http';
import { Observable, throwError } from 'rxjs';
import { apiRetry } from '../api-utils';
import { retry, catchError } from 'rxjs/operators';
import { MessageService } from '../services/message.service';

@Injectable()
export class ErrorInterceptor implements HttpInterceptor {

  constructor(private messageService: MessageService) { }

  intercept(request: HttpRequest<any>, next: HttpHandler): Observable<HttpEvent<any>> {
    return next.handle(request)
      .pipe(
        retry(apiRetry),
        catchError((error: HttpErrorResponse) => {
          let errorMsg: string;
          if (error.error instanceof ErrorEvent) {
            // Client side
            errorMsg = `Error: ${error.error.message}`;
          } else {
            // Server side
            errorMsg = `Error Status: ${error.status}\nMessage: ${error.message}`;
          }
          console.log(errorMsg);

          this.messageService.add(errorMsg);
          return throwError(errorMsg);
        })
      )
  }
}
