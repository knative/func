/**
 * Function code used on Update test. Basically the default function
 * implementation is replaced by this one. We test body responds properly
 * after function re-deployment
 *
 * @param context
 * @returns {{body: string}}
 */
function invoke(context) {
  return { body: 'HELLO NODE FUNCTION' }
}

module.exports = invoke;
