package ocxsql

import (
	"database/sql"
	"fmt"

	"github.com/mit-dci/opencx/match"
)

// PlaceOrder runs the queries which places an input order. Placing an individual order is atomic.
func (db *DB) PlaceOrder(order *match.LimitOrder) error {

	// Check that they have the balance for the order
	// if they do, place the order and update their balance

	// when placing an order subtract from the balance
	return nil
}

// runMatching is private since you don't really care about being able to call it from the outside, just to run it when certain things update
func (db *DB) runMatching(pair match.Pair) (err error) {

	tx, err := db.DBHandler.Begin()
	if err != nil {
		return fmt.Errorf("Error beginning transaction while updating deposits: \n%s", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
			err = fmt.Errorf("Error while running matching, this might be bad: \n%s", err)
			return
		}
		err = tx.Commit()
	}()

	if _, err = tx.Exec("USE " + db.orderSchema + ";"); err != nil {
		return
	}

	// First get all the prices so we have something to iterate through and match
	getPricesQuery := fmt.Sprintf("SELECT DISTINCT price FROM %s;", pair.String())
	rows, err := tx.Query(getPricesQuery)
	if err != nil {
		return
	}
	for rows.Next() {
		var price float64
		if err = rows.Scan(&price); err != nil {
			return
		}

		// this will select all sell side, ordered by time ascending so the earliest one will be at the front
		getSellSideQuery := fmt.Sprintf("SELECT name, orderID, side, amountHave, amountWant FROM %s WHERE price=%f AND side='%s' ORDER BY time ASC;", pair.String(), price, "sell")
		sellRows, sellQueryErr := tx.Query(getSellSideQuery)
		if err = sellQueryErr; err != nil {
			return
		}

		getBuySideQuery := fmt.Sprintf("SELECT name, orderID, amountHave, amountWant FROM %s WHERE price=%f AND side='%s' ORDER BY time ASC;", pair.String(), price, "buy")
		buyRows, buyQueryErr := tx.Query(getBuySideQuery)
		if err = buyQueryErr; err != nil {
			return
		}

		// loop through them both and make sure there are elements in both otherwise we're good
		for buyRows.Next() && sellRows.Next() {
			currBuyOrder := new(match.LimitOrder)
			currSellOrder := new(match.LimitOrder)
			if err = buyRows.Scan(&currBuyOrder.Client, &currBuyOrder.OrderID, &currBuyOrder.AmountHave, &currBuyOrder.AmountWant); err != nil {
				return
			}

			if err = sellRows.Scan(&currSellOrder.Client, &currSellOrder.OrderID, &currSellOrder.AmountHave, &currSellOrder.AmountWant); err != nil {
				return
			}

			// buying:
			// when we calculate price, could this conditional lead to some weird matching favoritism?
			if currBuyOrder.AmountHave > currSellOrder.AmountWant {

				// keep these to see if we can get any pennies off the order or something?? Isn't that illegal?
				// to see if there's a difference in price technically as well
				prevAmountHave := currSellOrder.AmountHave
				prevAmountWant := currSellOrder.AmountWant
				currBuyOrder.AmountHave -= currSellOrder.AmountWant
				currBuyOrder.AmountWant -= currSellOrder.AmountHave

				// update order with new amounts
				if err = db.UpdateOrderAmountsWithinTransaction(currBuyOrder, pair, tx); err != nil {
					return
				}
				// delete sell order
				if err = db.DeleteOrderWithinTransaction(currSellOrder, pair, tx); err != nil {
					return
				}

				// use the balance schema because we're ending with balance transactions
				if _, err = tx.Exec("USE " + db.balanceSchema + ";"); err != nil {
					return
				}

				// credit buyOrder client with sellOrder amountHave
				if err = db.UpdateBalanceWithinTransaction(currBuyOrder.Client, prevAmountHave, tx, pair.AssetWant.GetAssociatedCoinParam()); err != nil {
					return
				}
				// credit sellOrder client with buyorder amountWant
				if err = db.UpdateBalanceWithinTransaction(currSellOrder.Client, prevAmountWant, tx, pair.AssetHave.GetAssociatedCoinParam()); err != nil {
					return
				}
			} else if currBuyOrder.AmountHave < currSellOrder.AmountWant {

				// keep these to see if we can get any pennies off the order or something?? Isn't that illegal?
				// to see if there's a difference in price technically as well
				prevAmountHave := currBuyOrder.AmountHave
				prevAmountWant := currBuyOrder.AmountWant
				currSellOrder.AmountHave -= currBuyOrder.AmountWant
				currSellOrder.AmountWant -= currBuyOrder.AmountHave

				// update order with new amounts
				if err = db.UpdateOrderAmountsWithinTransaction(currSellOrder, pair, tx); err != nil {
					return
				}
				// delete buy order
				if err = db.DeleteOrderWithinTransaction(currBuyOrder, pair, tx); err != nil {
					return
				}

				// use the balance schema because we're ending with balance transactions
				if _, err = tx.Exec("USE " + db.balanceSchema + ";"); err != nil {
					return
				}

				// credit buyOrder client with sellOrder amountHave
				if err = db.UpdateBalanceWithinTransaction(currBuyOrder.Client, prevAmountWant, tx, pair.AssetWant.GetAssociatedCoinParam()); err != nil {
					return
				}
				// credit sellOrder client with buyorder amountWant
				if err = db.UpdateBalanceWithinTransaction(currSellOrder.Client, prevAmountHave, tx, pair.AssetHave.GetAssociatedCoinParam()); err != nil {
					return
				}
			} else if currBuyOrder.AmountHave == currSellOrder.AmountWant {

				// this is if they can perfectly fill each others orders

				// delete buy order
				if err = db.DeleteOrderWithinTransaction(currBuyOrder, pair, tx); err != nil {
					return
				}
				// delete sell order
				if err = db.DeleteOrderWithinTransaction(currSellOrder, pair, tx); err != nil {
					return
				}

				// use the balance schema because we're ending with balance transactions
				if _, err = tx.Exec("USE " + db.balanceSchema + ";"); err != nil {
					return
				}

				// credit buyOrder client with sellOrder amountHave
				if err = db.UpdateBalanceWithinTransaction(currBuyOrder.Client, currBuyOrder.AmountWant, tx, pair.AssetWant.GetAssociatedCoinParam()); err != nil {
					return
				}
				// credit sellOrder client with buyorder amountWant
				if err = db.UpdateBalanceWithinTransaction(currSellOrder.Client, currBuyOrder.AmountHave, tx, pair.AssetHave.GetAssociatedCoinParam()); err != nil {
					return
				}
			}
		}
	}
	return
}

// UpdateOrderAmountsWithinTransaction updates a single order within a sql transaction
func (db *DB) UpdateOrderAmountsWithinTransaction(order *match.LimitOrder, pair match.Pair, tx *sql.Tx) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("Error updating order within transaction: \n%s", err)
			return
		}
	}()

	updateOrderQuery := fmt.Sprintf("UPDATE %s SET amountHave=%d, amountWant=%d WHERE orderID='%s';", pair.String(), order.AmountHave, order.AmountWant, order.OrderID)
	if _, err = tx.Exec(updateOrderQuery); err != nil {
		return
	}
	return
}

// DeleteOrderWithinTransaction deletes an order within a transaction.
func (db *DB) DeleteOrderWithinTransaction(order *match.LimitOrder, pair match.Pair, tx *sql.Tx) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("Error deleting order within transaction: \n%s", err)
			return
		}
	}()

	deleteOrderQuery := fmt.Sprintf("DELETE FROM %s WHERE orderID='%s';", pair.String(), order.OrderID)
	if _, err = tx.Exec(deleteOrderQuery); err != nil {
		return
	}
	return
}

// CancelOrder runs the queries to cancel an order. Cancelling an individual order is atomic.
func (db *DB) CancelOrder(order *match.LimitOrder) error {

	// credit client with amounthave
	return nil
}

// GetPrice returns the price based on the orderbook
func (db *DB) GetPrice() error {
	return nil
}
