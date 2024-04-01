import React, { useState, useRef } from 'react';
import DatePicker from 'react-datepicker';
import 'react-datepicker/dist/react-datepicker.css';

function MyDatePicker() {
  const [selectedDate, setSelectedDate] = useState(null);
  const selectedDateRef = useRef(null); // Create a ref

  const handleDateChange = date => {
    setSelectedDate(date);
  };

  return (
    <div>
      <h2>Date Picker Example</h2>
      <DatePicker
        ref={selectedDateRef} // Assign the ref here
        selected={selectedDate}
        onChange={handleDateChange}
        dateFormat="dd/MM/yyyy"
        placeholderText="Select a date"
      />
    </div>
  );
}

export default MyDatePicker;