import React from 'react';
import DatePicker from 'react-datepicker';
import 'react-datepicker/dist/react-datepicker.css';
//date picker shit
const MyDatePicker = ({ selectedDate, onDateChange }) => {
  const handleDateChange = date => {
    onDateChange(date); // Call the parent component's function to update state
  };

  return (
    <div>
      <h2 className='DateTxt'>Date Picker</h2>
      <DatePicker
        selected={selectedDate}
        onChange={handleDateChange}
        dateFormat="MM/dd/yyyy"
        placeholderText="Select a date"
      />
    </div>
  );
}

export default MyDatePicker;
