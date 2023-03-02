import { Context, Invokable } from 'faas-js-runtime';

export const handle: Invokable = function (_: Context): string {
    return 'HELLO TYPESCRIPT FUNCTION';
};
