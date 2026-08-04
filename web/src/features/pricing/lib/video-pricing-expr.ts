export type VideoPriceItem = {
  id: string
  value: string
  pricePerSecond: string
}

export function buildVideoPricingExpression(
  durationPath: string,
  optionPath: string,
  exchangeRate: string,
  defaultDuration: string,
  maxDuration: string,
  items: VideoPriceItem[],
  fallbackItemId: string
): string | null {
  const normalizedDurationPath = durationPath.trim()
  const normalizedOptionPath = optionPath.trim()
  const rate = Number(exchangeRate)
  const durationDefault = Number(defaultDuration)
  const durationLimit = Number(maxDuration)
  const validItems = items.filter(
    (item) =>
      item.value.trim() &&
      item.pricePerSecond.trim() &&
      Number.isFinite(Number(item.pricePerSecond)) &&
      Number(item.pricePerSecond) >= 0
  )
  const optionValues = new Set(validItems.map((item) => item.value.trim()))
  if (
    !normalizedDurationPath ||
    !normalizedOptionPath ||
    !Number.isFinite(rate) ||
    rate <= 0 ||
    !Number.isFinite(durationDefault) ||
    durationDefault <= 0 ||
    durationDefault > durationLimit ||
    !Number.isFinite(durationLimit) ||
    durationLimit <= 0 ||
    durationLimit > 3600 ||
    validItems.length === 0 ||
    optionValues.size !== validItems.length
  ) {
    return null
  }

  const durationParam = `param(${JSON.stringify(normalizedDurationPath)})`
  const optionParam = `param(${JSON.stringify(normalizedOptionPath)})`
  const durationExpr = `min(max(${durationParam} == nil ? ${durationDefault} : number(${durationParam}), 0), ${durationLimit})`
  const costExpr = (item: VideoPriceItem, tierName = item.value.trim()) =>
    `tier(${JSON.stringify(tierName)}, ${durationExpr} * (${Number(item.pricePerSecond)} / ${rate}) * 1000000)`
  const fallbackItem =
    validItems.find((item) => item.id === fallbackItemId) || validItems[0]
  let expression = costExpr(
    fallbackItem,
    `unknown_${fallbackItem.value.trim()}`
  )

  for (let index = validItems.length - 1; index >= 0; index--) {
    const item = validItems[index]
    expression = `${optionParam} == ${JSON.stringify(item.value.trim())} ? ${costExpr(item)} : ${expression}`
  }
  return expression
}
