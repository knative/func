import { Context, Invokable } from 'faas-js-runtime';

export const handle: Invokable = function (context: Context): string {
    return 'HELLO TYPESCRIPT FUNCTION';
};